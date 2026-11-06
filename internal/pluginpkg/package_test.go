package pluginpkg

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testManifest = `id: example
name: Example
version: 1.0.0
type: workload
protocol_version: 1
min_agent_version: 0.1.0
platforms: [linux/amd64, windows/amd64]
entrypoint: bin/hitstor-plugin-example
`

func testKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv
}

func trustOnly(t *testing.T, key ed25519.PrivateKey, allowUnsigned bool) TrustPolicy {
	t.Helper()
	return TrustPolicy{Keys: []ed25519.PublicKey{key.Public().(ed25519.PublicKey)}, AllowUnsigned: allowUnsigned}
}

func buildTest(t *testing.T, key ed25519.PrivateKey) []byte {
	t.Helper()
	data, err := Build([]byte(testManifest), map[string][]byte{
		"bin/hitstor-plugin-example": []byte("binary"),
		"README.md":                  []byte("docs"),
	}, key)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return data
}

func readBytes(t *testing.T, data []byte) (*Package, error) {
	t.Helper()
	return Read(bytes.NewReader(data), int64(len(data)))
}

func TestBuildReadVerifyRoundTrip(t *testing.T) {
	key := testKey(t)
	pkg, err := readBytes(t, buildTest(t, key))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if pkg.Manifest.ID != "example" || pkg.Manifest.Version != "1.0.0" {
		t.Fatalf("manifest = %+v", pkg.Manifest)
	}
	if !pkg.Signed() || pkg.Signer() != EncodePublicKey(key.Public().(ed25519.PublicKey)) {
		t.Fatalf("signer = %q", pkg.Signer())
	}
	if err := pkg.Verify(trustOnly(t, key, false)); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	key := testKey(t)
	if !bytes.Equal(buildTest(t, key), buildTest(t, key)) {
		t.Fatal("two builds of the same input differ")
	}
}

func TestVerifyRejectsUntrustedKey(t *testing.T) {
	pkg, err := readBytes(t, buildTest(t, testKey(t)))
	if err != nil {
		t.Fatal(err)
	}
	// AllowUnsigned must not let a package signed by a stranger through.
	err = pkg.Verify(trustOnly(t, testKey(t), true))
	if !errors.Is(err, ErrUntrustedKey) {
		t.Fatalf("err = %v, want ErrUntrustedKey", err)
	}
}

func TestVerifyUnsigned(t *testing.T) {
	key := testKey(t)
	pkg, err := readBytes(t, buildTest(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.Verify(trustOnly(t, key, false)); !errors.Is(err, ErrUnsigned) {
		t.Fatalf("err = %v, want ErrUnsigned", err)
	}
	if err := pkg.Verify(trustOnly(t, key, true)); err != nil {
		t.Fatalf("AllowUnsigned: %v", err)
	}
}

// rewriteZip copies a package, letting edit replace entry contents.
func rewriteZip(t *testing.T, data []byte, edit func(name string, body []byte) []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		body, err := readEntry(f)
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		w.Write(edit(f.Name, body))
	}
	zw.Close()
	return buf.Bytes()
}

func TestVerifyDetectsTampering(t *testing.T) {
	key := testKey(t)
	tampered := rewriteZip(t, buildTest(t, key), func(name string, body []byte) []byte {
		if name == "bin/hitstor-plugin-example" {
			return []byte("malicious")
		}
		return body
	})
	pkg, err := readBytes(t, tampered)
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.Verify(trustOnly(t, key, false)); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("err = %v, want ErrBadSignature", err)
	}
}

func TestVerifyDetectsAddedEntry(t *testing.T) {
	key := testKey(t)
	data := buildTest(t, key)
	zr, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		body, _ := readEntry(f)
		w, _ := zw.Create(f.Name)
		w.Write(body)
	}
	w, _ := zw.Create("bin/extra.dll")
	w.Write([]byte("planted"))
	zw.Close()

	pkg, err := readBytes(t, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.Verify(trustOnly(t, key, false)); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("err = %v, want ErrBadSignature", err)
	}
}

func zipWith(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func TestReadRejectsUnsafeArchives(t *testing.T) {
	base := map[string]string{
		ManifestName:                 testManifest,
		"bin/hitstor-plugin-example": "binary",
	}
	cases := map[string]string{
		"parent escape":  "../evil",
		"nested escape":  "bin/../../evil",
		"absolute":       "/etc/evil",
		"windows drive":  "C:/evil",
		"backslash":      `bin\evil`,
		"not clean (./)": "./evil",
	}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			entries := map[string]string{bad: "x"}
			for k, v := range base {
				entries[k] = v
			}
			if _, err := readBytes(t, zipWith(t, entries)); err == nil {
				t.Fatalf("entry %q was accepted", bad)
			}
		})
	}
}

func TestReadRejectsSymlink(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{ManifestName: testManifest, "bin/hitstor-plugin-example": "binary"} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	hdr := &zip.FileHeader{Name: "bin/link"}
	hdr.SetMode(os.ModeSymlink | 0o777)
	w, _ := zw.CreateHeader(hdr)
	w.Write([]byte("/etc/passwd"))
	zw.Close()
	if _, err := readBytes(t, buf.Bytes()); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("err = %v, want regular-file rejection", err)
	}
}

func TestReadRequiresManifestAndEntrypoint(t *testing.T) {
	if _, err := readBytes(t, zipWith(t, map[string]string{"bin/hitstor-plugin-example": "x"})); err == nil {
		t.Fatal("package without manifest accepted")
	}
	if _, err := readBytes(t, zipWith(t, map[string]string{ManifestName: testManifest})); err == nil {
		t.Fatal("package without entrypoint accepted")
	}
	if _, err := readBytes(t, []byte("not a zip")); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestBuildRejectsReservedAndMissing(t *testing.T) {
	if _, err := Build([]byte(testManifest), map[string][]byte{SignatureName: nil, "bin/hitstor-plugin-example": nil}, nil); err == nil {
		t.Fatal("SIGNATURE entry accepted")
	}
	if _, err := Build([]byte(testManifest), map[string][]byte{"README.md": nil}, nil); err == nil {
		t.Fatal("missing entrypoint accepted")
	}
}

func TestExtract(t *testing.T) {
	pkg, err := readBytes(t, buildTest(t, testKey(t)))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "example")
	if err := pkg.Extract(dir); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	for _, name := range []string{ManifestName, "README.md", "bin/hitstor-plugin-example"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, SignatureName)); !os.IsNotExist(err) {
		t.Errorf("SIGNATURE was extracted")
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(filepath.Join(dir, "bin", "hitstor-plugin-example"))
		if fi.Mode().Perm()&0o100 == 0 {
			t.Errorf("entrypoint mode = %v, want executable", fi.Mode())
		}
	}
	if err := pkg.Extract(dir); err == nil {
		t.Fatal("Extract into an existing directory succeeded")
	}
}

func TestManifestValidation(t *testing.T) {
	cases := map[string]string{
		"bad id":          strings.Replace(testManifest, "id: example", "id: Example_1", 1),
		"bad version":     strings.Replace(testManifest, "version: 1.0.0", "version: one", 1),
		"bad type":        strings.Replace(testManifest, "type: workload", "type: hook", 1),
		"no protocol":     strings.Replace(testManifest, "protocol_version: 1", "protocol_version: 0", 1),
		"bad platform":    strings.Replace(testManifest, "linux/amd64", "linux", 1),
		"escaping entry":  strings.Replace(testManifest, "bin/hitstor-plugin-example", "../x", 1),
		"unknown field":   testManifest + "permissions: all\n",
		"bad min version": strings.Replace(testManifest, "0.1.0", "latest", 1),
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest([]byte(m)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	m, err := ParseManifest([]byte(testManifest))
	if err != nil {
		t.Fatal(err)
	}
	if !m.SupportsPlatform("windows", "amd64") || m.SupportsPlatform("linux", "arm64") {
		t.Fatal("SupportsPlatform wrong")
	}
}

func TestOfficialKeysParse(t *testing.T) {
	p, err := NewTrustPolicy([]string{EncodePublicKey(testKey(t).Public().(ed25519.PublicKey))}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Keys) != len(OfficialKeys)+1 {
		t.Fatalf("keys = %d", len(p.Keys))
	}
	if _, err := NewTrustPolicy([]string{"not-a-key"}, false); err == nil {
		t.Fatal("bad key accepted")
	}
}

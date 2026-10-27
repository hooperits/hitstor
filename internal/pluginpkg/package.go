package pluginpkg

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Limits guarding against hostile archives.
const (
	MaxEntries   = 1024
	MaxTotalSize = 512 << 20 // uncompressed bytes across all entries
)

// digestDomain separates package digests from any other ed25519 use of the
// same key.
const digestDomain = "hitstor-plugin-package-v1\n"

// signatureFile is the JSON stored in SIGNATURE.
type signatureFile struct {
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

// Package is a parsed plugin package held in memory.
type Package struct {
	Manifest *Manifest

	files     map[string][]byte // every entry except SIGNATURE
	signer    ed25519.PublicKey
	signature []byte
}

// Signed reports whether the package carries a SIGNATURE.
func (p *Package) Signed() bool { return p.signer != nil }

// Signer returns the base64 public key that signed the package, or "".
func (p *Package) Signer() string {
	if p.signer == nil {
		return ""
	}
	return EncodePublicKey(p.signer)
}

// Digest returns the canonical SHA-256 digest that the signature covers:
// every entry except SIGNATURE, sorted by name, as "name\nsha256hex\n".
func (p *Package) Digest() [32]byte {
	return digestOf(p.files)
}

func digestOf(files map[string][]byte) [32]byte {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)

	h := sha256.New()
	io.WriteString(h, digestDomain)
	for _, n := range names {
		sum := sha256.Sum256(files[n])
		io.WriteString(h, n+"\n"+hex.EncodeToString(sum[:])+"\n")
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// Verify checks the package against a trust policy.
func (p *Package) Verify(policy TrustPolicy) error {
	if p.signer == nil {
		if policy.AllowUnsigned {
			return nil
		}
		return ErrUnsigned
	}
	if !policy.trusts(p.signer) {
		return fmt.Errorf("%w (%s)", ErrUntrustedKey, p.Signer())
	}
	d := p.Digest()
	if !ed25519.Verify(p.signer, d[:], p.signature) {
		return ErrBadSignature
	}
	return nil
}

// Build creates a package from a manifest and its files. files maps entry
// names (slash-separated, relative) to contents and must include the
// manifest's entrypoint; it must not include plugin.yaml or SIGNATURE. When
// key is nil the package is left unsigned.
//
// Output is deterministic: entries are sorted and carry a fixed timestamp.
func Build(manifest []byte, files map[string][]byte, key ed25519.PrivateKey) ([]byte, error) {
	m, err := ParseManifest(manifest)
	if err != nil {
		return nil, err
	}
	all := map[string][]byte{ManifestName: manifest}
	for name, data := range files {
		if err := validEntryName(name); err != nil {
			return nil, fmt.Errorf("pluginpkg: %w", err)
		}
		if name == ManifestName || name == SignatureName {
			return nil, fmt.Errorf("pluginpkg: %s is reserved", name)
		}
		all[name] = data
	}
	if _, ok := all[m.Entrypoint]; !ok {
		return nil, fmt.Errorf("pluginpkg: entrypoint %q is missing from the package", m.Entrypoint)
	}

	if key != nil {
		d := digestOf(all)
		sig, err := json.MarshalIndent(signatureFile{
			Algorithm: "ed25519",
			PublicKey: EncodePublicKey(key.Public().(ed25519.PublicKey)),
			Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, d[:])),
		}, "", "  ")
		if err != nil {
			return nil, err
		}
		all[SignatureName] = append(sig, '\n')
	}

	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	modTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, n := range names {
		hdr := &zip.FileHeader{Name: n, Method: zip.Deflate, Modified: modTime}
		mode := os.FileMode(0o644)
		if n == m.Entrypoint {
			mode = 0o755
		}
		hdr.SetMode(mode)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(all[n]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ReadFile opens and parses a package file. It does not verify the
// signature; call Verify.
func ReadFile(path string) (*Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("pluginpkg: %w", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("pluginpkg: %w", err)
	}
	return Read(f, fi.Size())
}

// Read parses a package from r. It rejects archives with unsafe names,
// duplicates, links, or excessive size, and does not verify the signature.
func Read(r io.ReaderAt, size int64) (*Package, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("pluginpkg: not a plugin package: %w", err)
	}
	if len(zr.File) > MaxEntries {
		return nil, fmt.Errorf("pluginpkg: too many entries (%d)", len(zr.File))
	}

	files := make(map[string][]byte)
	var sigData []byte
	var total uint64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if err := validEntryName(f.Name); err != nil {
			return nil, fmt.Errorf("pluginpkg: %w", err)
		}
		if !f.Mode().IsRegular() {
			return nil, fmt.Errorf("pluginpkg: %q is not a regular file", f.Name)
		}
		if _, dup := files[f.Name]; dup || (f.Name == SignatureName && sigData != nil) {
			return nil, fmt.Errorf("pluginpkg: duplicate entry %q", f.Name)
		}
		total += f.UncompressedSize64
		if total > MaxTotalSize {
			return nil, fmt.Errorf("pluginpkg: package exceeds %d bytes", MaxTotalSize)
		}
		data, err := readEntry(f)
		if err != nil {
			return nil, err
		}
		if f.Name == SignatureName {
			sigData = data
			continue
		}
		files[f.Name] = data
	}

	manifest, ok := files[ManifestName]
	if !ok {
		return nil, fmt.Errorf("pluginpkg: %s is missing", ManifestName)
	}
	m, err := ParseManifest(manifest)
	if err != nil {
		return nil, err
	}
	if _, ok := files[m.Entrypoint]; !ok {
		return nil, fmt.Errorf("pluginpkg: entrypoint %q is missing from the package", m.Entrypoint)
	}

	p := &Package{Manifest: m, files: files}
	if sigData != nil {
		var sf signatureFile
		if err := json.Unmarshal(sigData, &sf); err != nil {
			return nil, fmt.Errorf("pluginpkg: parse %s: %w", SignatureName, err)
		}
		if sf.Algorithm != "ed25519" {
			return nil, fmt.Errorf("pluginpkg: unsupported signature algorithm %q", sf.Algorithm)
		}
		pub, err := ParsePublicKey(sf.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("pluginpkg: %s public key: %w", SignatureName, err)
		}
		sig, err := base64.StdEncoding.DecodeString(sf.Signature)
		if err != nil || len(sig) != ed25519.SignatureSize {
			return nil, fmt.Errorf("pluginpkg: %s: malformed signature", SignatureName)
		}
		p.signer, p.signature = pub, sig
	}
	return p, nil
}

func readEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("pluginpkg: open %q: %w", f.Name, err)
	}
	defer rc.Close()
	// Never trust the declared size alone: cap what is actually inflated.
	data, err := io.ReadAll(io.LimitReader(rc, int64(f.UncompressedSize64)+1))
	if err != nil {
		return nil, fmt.Errorf("pluginpkg: read %q: %w", f.Name, err)
	}
	if uint64(len(data)) != f.UncompressedSize64 {
		return nil, fmt.Errorf("pluginpkg: %q: size mismatch", f.Name)
	}
	return data, nil
}

// Extract writes the package contents (manifest included, SIGNATURE
// excluded) under dir, which must not exist yet. The entrypoint is made
// executable.
func (p *Package) Extract(dir string) error {
	if err := os.Mkdir(dir, 0o755); err != nil {
		return fmt.Errorf("pluginpkg: %w", err)
	}
	for name, data := range p.files {
		dst := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("pluginpkg: %w", err)
		}
		mode := os.FileMode(0o644)
		if name == p.Manifest.Entrypoint {
			mode = 0o755
		}
		if err := os.WriteFile(dst, data, mode); err != nil {
			return fmt.Errorf("pluginpkg: %w", err)
		}
	}
	return nil
}

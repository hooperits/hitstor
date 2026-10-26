//go:build linux

package agent

import (
	"fmt"
	"strings"
	"testing"
)

type fakeRunner struct {
	// responses keyed by "name arg0 arg1..."
	out map[string]string
	err map[string]error
}

func (f fakeRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	// prefix match for flexibility
	for k, v := range f.out {
		if key == k || strings.HasPrefix(key, k) {
			e := f.err[k]
			return []byte(v), e
		}
	}
	return nil, fmt.Errorf("unexpected command: %s", key)
}

func TestLookupMountParse(t *testing.T) {
	restore := setCmdRunner(fakeRunner{
		out: map[string]string{
			"findmnt -n -T /var/lib/app -o TARGET,SOURCE,FSTYPE": "/var/lib /dev/mapper/vg-data ext4",
		},
	})
	defer restore()

	mi, err := lookupMount("/var/lib/app")
	if err != nil {
		t.Fatal(err)
	}
	if mi.target != "/var/lib" || mi.source != "/dev/mapper/vg-data" || mi.fstype != "ext4" {
		t.Fatalf("%+v", mi)
	}
}

func TestParseLVMPath(t *testing.T) {
	vg, lv, err := parseLVMPath("/dev/mapper/vg0-root")
	if err != nil || vg != "vg0" || lv != "root" {
		t.Fatalf("mapper: %s/%s %v", vg, lv, err)
	}
	vg, lv, err = parseLVMPath("/dev/vgdata/lvapp")
	if err != nil || vg != "vgdata" || lv != "lvapp" {
		t.Fatalf("dev: %s/%s %v", vg, lv, err)
	}
	if _, _, err := parseLVMPath("/dev/sda1"); err == nil {
		t.Fatal("expected error for plain partition")
	}
}

func TestLooksLikeLVM(t *testing.T) {
	if !looksLikeLVM("/dev/mapper/vg-lv") {
		t.Fatal("mapper")
	}
	if !looksLikeLVM("/dev/myvg/mylv") {
		t.Fatal("vg/lv")
	}
	if looksLikeLVM("/dev/sda1") {
		t.Fatal("partition should not match")
	}
}

func TestSanitizeName(t *testing.T) {
	if sanitizeName("foo/bar") != "foo_bar" {
		t.Fatalf("%q", sanitizeName("foo/bar"))
	}
}

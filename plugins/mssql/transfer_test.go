package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// fakeDevice replays a script of commands and records completions.
type fakeDevice struct {
	cmds      []*vdCommand
	completed []uint32
	sizes     []uint32
	end       error // returned after the script; errVDClosed by default
}

func (d *fakeDevice) GetCommand() (*vdCommand, error) {
	if len(d.cmds) == 0 {
		if d.end != nil {
			return nil, d.end
		}
		return nil, errVDClosed
	}
	c := d.cmds[0]
	d.cmds = d.cmds[1:]
	return c, nil
}

func (d *fakeDevice) CompleteCommand(_ *vdCommand, code, n uint32) error {
	d.completed = append(d.completed, code)
	d.sizes = append(d.sizes, n)
	return nil
}

func write(s string) *vdCommand { return &vdCommand{code: vdcWrite, buf: []byte(s)} }
func read(n int) *vdCommand     { return &vdCommand{code: vdcRead, buf: make([]byte, n)} }

func TestBackupTransfer(t *testing.T) {
	dev := &fakeDevice{cmds: []*vdCommand{write("abc"), {code: vdcFlush}, write("def"), {code: vdcClearError}, {code: 99}}}
	var out bytes.Buffer
	n, err := backupTransfer(dev, &out)
	if err != nil || n != 6 || out.String() != "abcdef" {
		t.Fatalf("n=%d err=%v out=%q", n, err, out.String())
	}
	want := []uint32{errorSuccess, errorSuccess, errorSuccess, errorSuccess, errorNotSupported}
	for i, c := range want {
		if dev.completed[i] != c {
			t.Fatalf("completion %d = %d, want %d", i, dev.completed[i], c)
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("upload failed") }

func TestBackupTransferReportsWriteFailure(t *testing.T) {
	dev := &fakeDevice{cmds: []*vdCommand{write("abc"), write("def")}}
	_, err := backupTransfer(dev, failingWriter{})
	if err == nil || !strings.Contains(err.Error(), "upload failed") {
		t.Fatalf("err = %v", err)
	}
	// SQL Server is told about the failure so BACKUP aborts.
	if dev.completed[0] != errorDiskFull || dev.completed[1] != errorDiskFull {
		t.Fatalf("completions = %v", dev.completed)
	}
}

func TestBackupTransferAbort(t *testing.T) {
	dev := &fakeDevice{cmds: []*vdCommand{write("abc")}, end: errVDAborted}
	if _, err := backupTransfer(dev, &bytes.Buffer{}); !errors.Is(err, errVDAborted) {
		t.Fatalf("err = %v", err)
	}
}

func TestRestoreTransfer(t *testing.T) {
	dev := &fakeDevice{cmds: []*vdCommand{read(4), read(4), read(4), read(4)}}
	n, err := restoreTransfer(dev, strings.NewReader("0123456789"))
	if err != nil || n != 10 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	wantCodes := []uint32{errorSuccess, errorSuccess, errorHandleEOF, errorHandleEOF}
	wantSizes := []uint32{4, 4, 2, 0}
	for i := range wantCodes {
		if dev.completed[i] != wantCodes[i] || dev.sizes[i] != wantSizes[i] {
			t.Fatalf("completion %d = %d/%d, want %d/%d", i, dev.completed[i], dev.sizes[i], wantCodes[i], wantSizes[i])
		}
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("item corrupt") }

func TestRestoreTransferReportsReadFailure(t *testing.T) {
	dev := &fakeDevice{cmds: []*vdCommand{read(4), read(4)}}
	_, err := restoreTransfer(dev, failingReader{})
	if err == nil || !strings.Contains(err.Error(), "item corrupt") {
		t.Fatalf("err = %v", err)
	}
	if dev.completed[0] != errorReadFault || dev.completed[1] != errorReadFault {
		t.Fatalf("completions = %v", dev.completed)
	}
}

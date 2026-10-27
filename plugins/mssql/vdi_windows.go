//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SQL Server VDI COM interface (sqlvdi.dll), from vdi.h / vdiguid.h /
// vdierror.h in Microsoft's sql-server-samples.
var (
	// CLSID_MSSQL_ClientVirtualDeviceSet is defined as IID_IClientVirtualDeviceSet.
	clsidClientVirtualDeviceSet = windows.GUID{Data1: 0x40700425, Data2: 0x0080, Data3: 0x11d2, Data4: [8]byte{0x85, 0x1f, 0x00, 0xc0, 0x4f, 0xc2, 0x17, 0x59}}
	iidClientVirtualDeviceSet2  = windows.GUID{Data1: 0xd0e6eb07, Data2: 0x7a62, Data3: 0x11d2, Data4: [8]byte{0x85, 0x73, 0x00, 0xc0, 0x4f, 0xc2, 0x17, 0x59}}

	procCoCreateInstance = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
)

const (
	clsctxInprocServer = 0x1
	infinite           = 0xFFFFFFFF

	vdETimeout = 0x80770003
	vdEAbort   = 0x80770004
	vdEClose   = 0x8077000e
)

// Vtable slots. IUnknown takes 0-2.
const (
	slotRelease = 2

	// IClientVirtualDeviceSet2
	slotGetConfiguration = 4
	slotOpenDevice       = 5
	slotClose            = 6
	slotCreateEx         = 11

	// IClientVirtualDevice
	slotGetCommand      = 3
	slotCompleteCommand = 4
)

// vdConfig mirrors struct VDConfig.
type vdConfig struct {
	DeviceCount           uint32
	Features              uint32
	PrefixZoneSize        uint32
	Alignment             uint32
	SoftFileMarkBlockSize uint32
	EOMWarningSize        uint32
	ServerTimeOut         uint32
	BlockSize             uint32
	MaxIODepth            uint32
	MaxTransferSize       uint32
	BufferAreaSize        uint32
}

// vdcCommand mirrors struct VDC_Command.
type vdcCommand struct {
	CommandCode uint32
	Size        uint32
	Position    uint64
	Buffer      *byte
}

// comObject is a COM interface pointer: its first word points at the vtable.
type comObject struct {
	vtbl *[16]uintptr
}

// call invokes a vtable method. Callers pass Go pointers converted to
// uintptr (out parameters, structs); uintptrescapes moves them to the heap
// and keeps them alive for the duration of the call, as LazyProc.Call does.
//
//go:uintptrescapes
func (o *comObject) call(slot int, args ...uintptr) uint32 {
	r, _, _ := syscall.SyscallN(o.vtbl[slot], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return uint32(r)
}

func (o *comObject) release() { o.call(slotRelease) }

type hresultError uint32

func (e hresultError) Error() string { return fmt.Sprintf("VDI error 0x%08X", uint32(e)) }

func check(hr uint32) error {
	if int32(hr) >= 0 {
		return nil
	}
	return hresultError(hr)
}

// utf16 converts s for a COM call; "" becomes a NULL pointer. Keep the
// result referenced until the call returns (runtime.KeepAlive), since a
// uintptr alone does not keep the memory alive.
func utf16(s string) (*uint16, error) {
	if s == "" {
		return nil, nil
	}
	return windows.UTF16PtrFromString(s)
}

// vdiDevice implements vdDevice over IClientVirtualDevice.
type vdiDevice struct{ obj *comObject }

func (d *vdiDevice) GetCommand() (*vdCommand, error) {
	var c *vdcCommand
	switch hr := d.obj.call(slotGetCommand, infinite, uintptr(unsafe.Pointer(&c))); hr {
	case 0:
	case vdEClose:
		return nil, errVDClosed
	case vdEAbort:
		return nil, errVDAborted
	default:
		return nil, check(hr)
	}
	cmd := &vdCommand{code: c.CommandCode, position: c.Position, handle: uintptr(unsafe.Pointer(c))}
	if c.Buffer != nil && c.Size > 0 {
		cmd.buf = unsafe.Slice(c.Buffer, c.Size)
	}
	return cmd, nil
}

func (d *vdiDevice) CompleteCommand(cmd *vdCommand, code, transferred uint32) error {
	return check(d.obj.call(slotCompleteCommand, cmd.handle, uintptr(code), uintptr(transferred), 0))
}

// configTimeout bounds how long SQL Server may take to open the device.
const configTimeout = 2 * time.Minute

func newDeviceName() string {
	var b [12]byte
	rand.Read(b[:])
	return "HitStor_" + hex.EncodeToString(b[:])
}

// withVirtualDevice creates a virtual device set on the given instance (""
// for the default instance), runs exec with the device name on another
// connection, and serves the device with transfer until SQL Server closes
// it. All COM calls happen on one locked OS thread.
func withVirtualDevice(ctx context.Context, instance string, exec func(ctx context.Context, device string) error, transfer func(vdDevice) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil {
		return fmt.Errorf("COM init: %w", err)
	}
	defer windows.CoUninitialize()

	var set *comObject
	r, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidClientVirtualDeviceSet)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidClientVirtualDeviceSet2)), uintptr(unsafe.Pointer(&set)))
	if err := check(uint32(r)); err != nil {
		return fmt.Errorf("create VDI device set (is sqlvdi.dll registered?): %w", err)
	}
	defer set.release()

	name := newDeviceName()
	inst, err := utf16(instance)
	if err != nil {
		return err
	}
	namePtr, err := utf16(name)
	if err != nil {
		return err
	}
	defer runtime.KeepAlive(inst)
	defer runtime.KeepAlive(namePtr)
	cfg := vdConfig{DeviceCount: 1}
	if err := check(set.call(slotCreateEx, uintptr(unsafe.Pointer(inst)), uintptr(unsafe.Pointer(namePtr)), uintptr(unsafe.Pointer(&cfg)))); err != nil {
		return fmt.Errorf("create virtual device on instance %q: %w", instance, err)
	}
	closed := false
	closeSet := func() {
		if !closed {
			set.call(slotClose)
			closed = true
		}
	}
	defer closeSet()

	sqlCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sqlDone := make(chan error, 1)
	go func() { sqlDone <- exec(sqlCtx, name) }()

	// Wait for SQL Server to open the device, noticing an early SQL failure
	// (permissions, a log backup in SIMPLE recovery, ...) instead of waiting
	// out the timeout.
	deadline := time.Now().Add(configTimeout)
	for {
		hr := set.call(slotGetConfiguration, 1000, uintptr(unsafe.Pointer(&cfg)))
		if hr == 0 {
			break
		}
		if hr != vdETimeout {
			closeSet()
			cancel()
			if serr := <-sqlDone; serr != nil {
				return serr
			}
			return fmt.Errorf("wait for SQL Server: %w", check(hr))
		}
		select {
		case serr := <-sqlDone:
			closeSet()
			if serr == nil {
				serr = errors.New("SQL Server finished without using the virtual device")
			}
			return serr
		default:
		}
		if time.Now().After(deadline) {
			closeSet()
			cancel()
			<-sqlDone
			return fmt.Errorf("SQL Server did not open the virtual device within %s", configTimeout)
		}
	}

	var dev *comObject
	if err := check(set.call(slotOpenDevice, uintptr(unsafe.Pointer(namePtr)), uintptr(unsafe.Pointer(&dev)))); err != nil {
		closeSet()
		cancel()
		<-sqlDone
		return fmt.Errorf("open virtual device: %w", err)
	}
	terr := transfer(&vdiDevice{obj: dev})
	closeSet()
	serr := <-sqlDone
	switch {
	case terr != nil && serr != nil && !errors.Is(terr, errVDAborted):
		return fmt.Errorf("%w (SQL Server: %v)", terr, serr)
	case serr != nil:
		return serr
	default:
		return terr
	}
}

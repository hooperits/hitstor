package main

import (
	"errors"
	"fmt"
	"io"
)

// VDI command codes (vdi.h, enum VDCommands).
const (
	vdcRead       = 1
	vdcWrite      = 2
	vdcClearError = 3
	vdcFlush      = 12
)

// Win32 completion codes SQL Server expects from CompleteCommand.
const (
	errorSuccess      = 0
	errorReadFault    = 30
	errorHandleEOF    = 38
	errorNotSupported = 50
	errorDiskFull     = 112
)

var (
	// errVDClosed is GetCommand's normal end: SQL Server closed the device.
	errVDClosed = errors.New("virtual device closed")
	// errVDAborted means the operation was aborted (SQL error or SignalAbort).
	errVDAborted = errors.New("virtual device aborted")
)

// vdCommand is one I/O request from SQL Server. buf aliases shared memory
// and is valid only until the command is completed.
type vdCommand struct {
	code     uint32
	buf      []byte
	position uint64
	handle   uintptr // the native VDC_Command*
}

// vdDevice is the client side of one virtual device.
type vdDevice interface {
	// GetCommand waits for the next request; errVDClosed ends the transfer.
	GetCommand() (*vdCommand, error)
	CompleteCommand(cmd *vdCommand, code uint32, transferred uint32) error
}

// backupTransfer copies what SQL Server writes to the device into w until
// the device closes. A write error is reported to SQL Server (which then
// fails the BACKUP) and returned once the device closes.
func backupTransfer(dev vdDevice, w io.Writer) (int64, error) {
	var total int64
	var werr error
	for {
		cmd, err := dev.GetCommand()
		if errors.Is(err, errVDClosed) {
			return total, werr
		}
		if err != nil {
			if werr != nil {
				return total, werr
			}
			return total, err
		}
		code, n := uint32(errorSuccess), uint32(0)
		switch cmd.code {
		case vdcWrite:
			if werr != nil {
				code = errorDiskFull
				break
			}
			m, err := w.Write(cmd.buf)
			n = uint32(m)
			total += int64(m)
			if err != nil {
				werr = fmt.Errorf("store backup data: %w", err)
				code = errorDiskFull
			}
		case vdcFlush, vdcClearError:
		default:
			code = errorNotSupported
		}
		if err := dev.CompleteCommand(cmd, code, n); err != nil {
			return total, fmt.Errorf("complete VDI command: %w", err)
		}
	}
}

// restoreTransfer feeds r to SQL Server's reads until the device closes.
// Reads past the end of r get ERROR_HANDLE_EOF, as a file would.
func restoreTransfer(dev vdDevice, r io.Reader) (int64, error) {
	var total int64
	var rerr error
	eof := false
	for {
		cmd, err := dev.GetCommand()
		if errors.Is(err, errVDClosed) {
			return total, rerr
		}
		if err != nil {
			if rerr != nil {
				return total, rerr
			}
			return total, err
		}
		code, n := uint32(errorSuccess), uint32(0)
		switch cmd.code {
		case vdcRead:
			switch {
			case rerr != nil:
				code = errorReadFault
			case eof:
				code = errorHandleEOF
			default:
				m, err := io.ReadFull(r, cmd.buf)
				n = uint32(m)
				total += int64(m)
				switch {
				case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
					eof = true
					code = errorHandleEOF
				case err != nil:
					rerr = fmt.Errorf("read backup data: %w", err)
					code = errorReadFault
				}
			}
		case vdcFlush, vdcClearError:
		default:
			code = errorNotSupported
		}
		if err := dev.CompleteCommand(cmd, code, n); err != nil {
			return total, fmt.Errorf("complete VDI command: %w", err)
		}
	}
}

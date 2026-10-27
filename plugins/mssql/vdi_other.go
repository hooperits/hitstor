//go:build !windows

package main

import (
	"context"
	"errors"
)

// withVirtualDevice is only available on Windows, where SQL Server's VDI
// (sqlvdi.dll) lives.
func withVirtualDevice(context.Context, string, func(context.Context, string) error, func(vdDevice) error) error {
	return errors.New("SQL Server VDI backup requires Windows")
}

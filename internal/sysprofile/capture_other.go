//go:build !linux

package sysprofile

import (
	"context"
	"os"
	"runtime"
	"time"
)

// Capture returns a minimal profile on platforms where HitStor does not yet
// model the storage layout. Bare-metal restore is Linux-only for now; Windows
// layout capture arrives with the WinPE recovery media.
func Capture(ctx context.Context) (*Profile, error) {
	profile := &Profile{
		CapturedAt: time.Now().UTC(),
		Arch:       runtime.GOARCH,
		Warnings: []string{
			"system layout capture is not implemented on " + runtime.GOOS,
		},
	}
	if hostname, err := os.Hostname(); err == nil {
		profile.Hostname = hostname
	}
	return profile, nil
}

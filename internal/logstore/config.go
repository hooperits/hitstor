package logstore

import (
	"runtime"
)

// Config controls in-memory retention and on-disk rotation.
type Config struct {
	MaxEntries     int
	FileEnabled    bool
	FilePath       string
	RotateMaxMB    int
	RotateMaxFiles int
}

// DefaultConfig returns platform-appropriate defaults.
func DefaultConfig() Config {
	path := "/var/log/hitstor/server.log"
	if runtime.GOOS == "windows" {
		path = `C:\ProgramData\HitStor\logs\server.log`
	}
	return Config{
		MaxEntries:     5000,
		FileEnabled:    true,
		FilePath:       path,
		RotateMaxMB:    50,
		RotateMaxFiles: 5,
	}
}

// Normalize applies defaults and clamps values.
func (c Config) Normalize() Config {
	d := DefaultConfig()
	if c.MaxEntries <= 0 {
		c.MaxEntries = d.MaxEntries
	}
	if c.MaxEntries < 500 {
		c.MaxEntries = 500
	}
	if c.MaxEntries > 50000 {
		c.MaxEntries = 50000
	}
	if c.FilePath == "" {
		c.FilePath = d.FilePath
	}
	if c.RotateMaxMB <= 0 {
		c.RotateMaxMB = d.RotateMaxMB
	}
	if c.RotateMaxMB > 500 {
		c.RotateMaxMB = 500
	}
	if c.RotateMaxFiles <= 0 {
		c.RotateMaxFiles = d.RotateMaxFiles
	}
	if c.RotateMaxFiles > 20 {
		c.RotateMaxFiles = 20
	}
	return c
}

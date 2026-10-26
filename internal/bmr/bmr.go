// Package bmr drives bare-metal recovery media: checking that a host can build
// it, writing the ReaR configuration that wires ReaR's recovery run to HitStor,
// and running the build itself.
//
// The work necessarily happens ON the protected host. Rescue media embeds that
// host's kernel, its drivers, and its disk layout, so it cannot be produced
// server-side from a backup alone. Both `hitstor` and `hitstor-agent` expose
// these operations, and the agent's BuildRecoveryMedia RPC calls into this
// package directly.
//
// Linux only. Windows recovery media (WinPE) arrives in a later phase; on other
// platforms Check reports the host as not ready and the build refuses to run.
package bmr

import "time"

// Readiness reports whether a host can build bare-metal recovery media, and
// what is missing when it cannot.
//
// It is deliberately a report rather than an error: an operator planning for a
// disaster wants the whole list of gaps at once, not the first one.
type Readiness struct {
	// Ready is true when media can be built right now.
	Ready bool `json:"ready"`

	// Platform is the host OS. Anything but "linux" is unsupported this phase.
	Platform string `json:"platform"`

	// ReaRInstalled reports whether the `rear` binary was found on PATH.
	ReaRInstalled bool   `json:"rear_installed"`
	ReaRVersion   string `json:"rear_version,omitempty"`
	ReaRPath      string `json:"rear_path,omitempty"`

	// Root is true when the process can run ReaR, which needs raw device access.
	Root bool `json:"root"`

	// Tools lists the external commands ReaR and layout capture depend on, in
	// a stable order, each marked found or missing.
	Tools []ToolStatus `json:"tools"`

	// Blockers are the reasons Ready is false, phrased for an operator.
	Blockers []string `json:"blockers,omitempty"`

	// Warnings are degradations that do not block a build but will produce
	// less capable media (no UEFI entries recorded, no LVM layout, and so on).
	Warnings []string `json:"warnings,omitempty"`

	CheckedAt time.Time `json:"checked_at"`
}

// ToolStatus is one external command's availability.
type ToolStatus struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
	// Required marks tools without which media cannot be built at all.
	Required bool `json:"required"`
	Found    bool `json:"found"`
	// Purpose explains, to an operator reading a readiness report, why HitStor
	// wants this tool.
	Purpose string `json:"purpose"`
}

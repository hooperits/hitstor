// Package users holds the dashboard's user accounts: roles scoped to
// agents, passwords (argon2id), TOTP second factor with recovery codes,
// sessions and an audit log. Accounts live in an encrypted file next to the
// agent registry, so the console needs no directory service.
package users

import (
	"slices"
	"time"
)

// Role is a set of permissions a grant gives.
type Role string

// Built-in roles, after the usual split in backup consoles: administrators,
// operators who run backups, operators who restore, and viewers.
const (
	RoleAdmin           Role = "admin"
	RoleBackupOperator  Role = "backup_operator"
	RoleRestoreOperator Role = "restore_operator"
	RoleViewer          Role = "viewer"
)

// Roles lists the built-in roles in display order.
var Roles = []Role{RoleAdmin, RoleBackupOperator, RoleRestoreOperator, RoleViewer}

// Permission is what a request needs.
type Permission string

const (
	// PermView reads agents, jobs, snapshots and history.
	PermView Permission = "view"
	// PermBackup creates, edits, runs and cancels backup jobs, and browses
	// an agent to choose what to back up.
	PermBackup Permission = "backup"
	// PermRestore browses backed-up contents and restores them.
	PermRestore Permission = "restore"
	// PermAdmin manages users, settings, repositories, plugins and agents.
	PermAdmin Permission = "admin"
)

var rolePerms = map[Role][]Permission{
	RoleAdmin:           {PermView, PermBackup, PermRestore, PermAdmin},
	RoleBackupOperator:  {PermView, PermBackup},
	RoleRestoreOperator: {PermView, PermRestore},
	RoleViewer:          {PermView},
}

// ValidRole reports whether r is a built-in role.
func ValidRole(r Role) bool { _, ok := rolePerms[r]; return ok }

// Grant gives a role over some agents: those listed, or every agent when
// the list is empty. Administrators always act on every agent.
type Grant struct {
	Role   Role     `json:"role"`
	Agents []string `json:"agents,omitempty"`
}

func (g Grant) allAgents() bool { return g.Role == RoleAdmin || len(g.Agents) == 0 }

// User is a dashboard account.
type User struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"` // lowercase, unique
	DisplayName string  `json:"display_name,omitempty"`
	Email       string  `json:"email,omitempty"` // lowercase, unique; where account emails go
	Grants      []Grant `json:"grants"`
	Disabled    bool    `json:"disabled,omitempty"`

	PasswordHash       string    `json:"password_hash"`
	PasswordChangedAt  time.Time `json:"password_changed_at"`
	MustChangePassword bool      `json:"must_change_password,omitempty"`

	// TOTP second factor. TOTPLastStep is the time step of the last code
	// accepted: a code is good once.
	TOTPSecret    string   `json:"totp_secret,omitempty"`
	TOTPEnabled   bool     `json:"totp_enabled,omitempty"`
	TOTPLastStep  int64    `json:"totp_last_step,omitempty"`
	RecoveryCodes []string `json:"recovery_codes,omitempty"` // keyed hashes, one use each

	FailedLogins int        `json:"failed_logins,omitempty"`
	LockedUntil  *time.Time `json:"locked_until,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

// IsAdmin reports whether u holds the administrator role.
func (u *User) IsAdmin() bool {
	for _, g := range u.Grants {
		if g.Role == RoleAdmin {
			return true
		}
	}
	return false
}

// Can reports whether u has permission p on agent. An empty agent asks
// whether u has p on any agent at all.
func (u *User) Can(p Permission, agent string) bool {
	if u == nil || u.Disabled {
		return false
	}
	for _, g := range u.Grants {
		if !slices.Contains(rolePerms[g.Role], p) {
			continue
		}
		if agent == "" || g.allAgents() || slices.Contains(g.Agents, agent) {
			return true
		}
	}
	return false
}

// CanAll reports whether u has permission p on every agent, as resources
// that span agents (repositories, server logs) require.
func (u *User) CanAll(p Permission) bool {
	if u == nil || u.Disabled {
		return false
	}
	for _, g := range u.Grants {
		if slices.Contains(rolePerms[g.Role], p) && g.allAgents() {
			return true
		}
	}
	return false
}

// Agents returns the agents u has permission p on; all is true when that is
// every agent.
func (u *User) Agents(p Permission) (all bool, agents map[string]bool) {
	agents = map[string]bool{}
	if u == nil || u.Disabled {
		return false, agents
	}
	for _, g := range u.Grants {
		if !slices.Contains(rolePerms[g.Role], p) {
			continue
		}
		if g.allAgents() {
			return true, agents
		}
		for _, a := range g.Agents {
			agents[a] = true
		}
	}
	return false, agents
}

// Locked reports whether failed logins have locked u at now.
func (u *User) Locked(now time.Time) bool {
	return u.LockedUntil != nil && now.Before(*u.LockedUntil)
}

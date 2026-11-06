package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/hooperits/hitstor/internal/users"
)

// newServeUserCmd manages dashboard users from the server console. It is
// the way in when no administrator can sign in (lost phone, locked out).
func newServeUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage dashboard users, their roles and two-step verification",
		Long: `Manage dashboard users from the server console.

Roles: admin, backup_operator, restore_operator, viewer. Give a role on
some agents with --grant role=agent1,agent2, or on every agent with
--grant role. Administrators always cover every agent.

New users and password resets print a temporary password; its owner
chooses their own and sets up an authenticator app at the next sign-in.`,
	}
	cmd.AddCommand(newServeUserAddCmd())
	cmd.AddCommand(newServeUserListCmd())
	cmd.AddCommand(newServeUserGrantCmd())
	cmd.AddCommand(newServeUserPasswdCmd())
	cmd.AddCommand(newServeUserReset2FACmd())
	cmd.AddCommand(newServeUserEnableCmd(true))
	cmd.AddCommand(newServeUserEnableCmd(false))
	cmd.AddCommand(newServeUserRemoveCmd())
	return cmd
}

// openUsers opens the users file and audit log next to the agent registry.
func openUsers() (*users.Store, *users.AuditLog, func(), error) {
	repo, be, err := openRepository()
	if err != nil {
		return nil, nil, nil, err
	}
	basePath, err := backendBasePath(be)
	if err != nil {
		be.Close()
		return nil, nil, nil, err
	}
	store, err := users.Open(basePath, repo.Key().EncryptKey)
	if err != nil {
		be.Close()
		return nil, nil, nil, fmt.Errorf("load dashboard users: %w", err)
	}
	audit, err := users.OpenAuditLog(basePath)
	if err != nil {
		be.Close()
		return nil, nil, nil, fmt.Errorf("open audit log: %w", err)
	}
	return store, audit, func() { be.Close() }, nil
}

// consoleAudit records a change made from the console.
func consoleAudit(audit *users.AuditLog, action, target, detail string) {
	audit.Append(users.AuditEvent{User: "console", Action: action, Target: target, OK: true, Detail: detail})
}

// parseGrants reads --grant values: "role" or "role=agent1,agent2".
func parseGrants(values []string) ([]users.Grant, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("give at least one --grant (for example --grant viewer)")
	}
	var grants []users.Grant
	for _, v := range values {
		role, agents, _ := strings.Cut(v, "=")
		g := users.Grant{Role: users.Role(strings.TrimSpace(role))}
		for _, a := range strings.Split(agents, ",") {
			if a = strings.TrimSpace(a); a != "" {
				g.Agents = append(g.Agents, a)
			}
		}
		grants = append(grants, g)
	}
	return grants, nil
}

func grantsSummary(grants []users.Grant) string {
	var parts []string
	for _, g := range grants {
		if g.Role == users.RoleAdmin || len(g.Agents) == 0 {
			parts = append(parts, string(g.Role)+" (all agents)")
		} else {
			parts = append(parts, string(g.Role)+" ("+strings.Join(g.Agents, ", ")+")")
		}
	}
	return strings.Join(parts, "; ")
}

func printTemporaryPassword(name, password string) {
	fmt.Fprintf(os.Stdout, "Temporary password for %s: %s\n", name, password)
	fmt.Fprintln(os.Stdout, "It works once: at the next sign-in they choose their own password and set up an authenticator app.")
}

func newServeUserAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a dashboard user",
		Example: `  hitstor serve user add maria --grant admin
  hitstor serve user add ops1 --grant backup_operator=web01,db01 --grant viewer`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			values, _ := cmd.Flags().GetStringArray("grant")
			grants, err := parseGrants(values)
			if err != nil {
				return err
			}
			display, _ := cmd.Flags().GetString("display-name")
			email, _ := cmd.Flags().GetString("email")
			store, audit, done, err := openUsers()
			if err != nil {
				return err
			}
			defer done()
			password := users.TemporaryPassword()
			u, err := store.Add(args[0], display, email, password, grants, true)
			if err != nil {
				return err
			}
			consoleAudit(audit, "user.add", u.Username, grantsSummary(u.Grants))
			fmt.Fprintf(os.Stdout, "User %s created: %s\n", u.Username, grantsSummary(u.Grants))
			printTemporaryPassword(u.Username, password)
			return nil
		},
	}
	cmd.Flags().StringArray("grant", nil, `role, or role=agent1,agent2 (repeatable)`)
	cmd.Flags().String("display-name", "", "full name shown in the dashboard")
	cmd.Flags().String("email", "", "address for password reset links (the dashboard sends invitations itself)")
	return cmd
}

func newServeUserListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List dashboard users",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, done, err := openUsers()
			if err != nil {
				return err
			}
			defer done()
			list := store.List()
			if len(list) == 0 {
				fmt.Fprintln(os.Stdout, "No dashboard users yet. The first administrator is created at /setup in the dashboard, or with 'hitstor serve user add <name> --grant admin'.")
				return nil
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "USER\tEMAIL\tROLES\tTWO-STEP\tSTATUS\tLAST SIGN-IN")
			for _, u := range list {
				twoStep := "pending"
				if u.TOTPEnabled {
					twoStep = "on"
				}
				status := "active"
				switch {
				case u.Disabled:
					status = "disabled"
				case u.Locked(time.Now()):
					status = "locked"
				case u.MustChangePassword:
					status = "must change password"
				}
				last := "never"
				if u.LastLoginAt != nil {
					last = u.LastLoginAt.Local().Format("2006-01-02 15:04")
				}
				email := u.Email
				if email == "" {
					email = "-"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", u.Username, email, grantsSummary(u.Grants), twoStep, status, last)
			}
			return tw.Flush()
		},
	}
}

func newServeUserGrantCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "grant <name>",
		Short:   "Replace a user's roles",
		Example: `  hitstor serve user grant ops1 --grant restore_operator=db01`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			values, _ := cmd.Flags().GetStringArray("grant")
			grants, err := parseGrants(values)
			if err != nil {
				return err
			}
			store, audit, done, err := openUsers()
			if err != nil {
				return err
			}
			defer done()
			if err := store.SetGrants(args[0], grants); err != nil {
				return err
			}
			consoleAudit(audit, "user.roles", args[0], grantsSummary(grants))
			fmt.Fprintf(os.Stdout, "Roles of %s: %s\n", args[0], grantsSummary(grants))
			return nil
		},
	}
	cmd.Flags().StringArray("grant", nil, `role, or role=agent1,agent2 (repeatable)`)
	return cmd
}

func newServeUserPasswdCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "passwd <name>",
		Short: "Reset a user's password to a temporary one and end their sessions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, audit, done, err := openUsers()
			if err != nil {
				return err
			}
			defer done()
			password := users.TemporaryPassword()
			if err := store.SetPassword(args[0], password, true, ""); err != nil {
				return err
			}
			consoleAudit(audit, "user.password-reset", args[0], "")
			printTemporaryPassword(args[0], password)
			return nil
		},
	}
}

func newServeUserReset2FACmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset-2fa <name>",
		Short: "Remove a user's authenticator app and recovery codes; they set it up again at the next sign-in",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, audit, done, err := openUsers()
			if err != nil {
				return err
			}
			defer done()
			if err := store.ResetSecondFactor(args[0]); err != nil {
				return err
			}
			consoleAudit(audit, "user.second-factor-reset", args[0], "")
			fmt.Fprintf(os.Stdout, "Two-step verification of %s reset. They set up an authenticator app at the next sign-in.\n", args[0])
			return nil
		},
	}
}

func newServeUserEnableCmd(enable bool) *cobra.Command {
	use, short, action := "disable <name>", "Stop a user from signing in and end their sessions", "user.disable"
	if enable {
		use, short, action = "enable <name>", "Let a disabled or locked-out user sign in again", "user.enable"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, audit, done, err := openUsers()
			if err != nil {
				return err
			}
			defer done()
			if err := store.SetDisabled(args[0], !enable); err != nil {
				return err
			}
			consoleAudit(audit, action, args[0], "")
			if enable {
				fmt.Fprintf(os.Stdout, "User %s enabled.\n", args[0])
			} else {
				fmt.Fprintf(os.Stdout, "User %s disabled.\n", args[0])
			}
			return nil
		},
	}
}

func newServeUserRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a dashboard user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, audit, done, err := openUsers()
			if err != nil {
				return err
			}
			defer done()
			if err := store.Remove(args[0]); err != nil {
				return err
			}
			consoleAudit(audit, "user.remove", args[0], "")
			fmt.Fprintf(os.Stdout, "User %s removed.\n", args[0])
			return nil
		},
	}
}

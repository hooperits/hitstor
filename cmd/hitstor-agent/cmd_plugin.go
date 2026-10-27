package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hooperits/hitstor/internal/agent"
	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/pluginmgr"
	"github.com/hooperits/hitstor/internal/pluginpkg"
)

// pluginStoreFlag overrides the plugin store directory.
var pluginStoreFlag string

func newPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Install and manage workload plugins on this agent",
		Long: `Install and manage workload plugins on this agent.

Plugins are signed .hpk packages that add workloads (for example a database
engine) to this agent. Packages must be signed by a trusted key: the official
HitStor key, or one listed in plugin_trusted_keys.`,
	}
	cmd.PersistentFlags().StringVar(&pluginStoreFlag, "store", "", "plugin store directory (default: plugin_store_dir or "+agent.DefaultPluginStoreDir+")")

	var allowDowngrade bool
	install := &cobra.Command{
		Use:   "install <package.hpk>",
		Short: "Install a plugin package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginInstall(cmd, args[0], pluginmgr.InstallOptions{})
		},
	}
	upgrade := &cobra.Command{
		Use:   "upgrade <package.hpk>",
		Short: "Replace an installed plugin with another version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginInstall(cmd, args[0], pluginmgr.InstallOptions{Upgrade: true, AllowDowngrade: allowDowngrade})
		},
	}
	upgrade.Flags().BoolVar(&allowDowngrade, "allow-downgrade", false, "allow installing an older version")

	list := &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newPluginManager()
			if err != nil {
				return err
			}
			plugins, err := mgr.List()
			if err != nil {
				return err
			}
			if jsonFlag {
				return writePluginJSON(plugins)
			}
			if len(plugins) == 0 {
				fmt.Printf("No plugins installed in %s\n", mgr.Dir())
				return nil
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tVERSION\tSTATUS\tSIGNED\tNAME")
			for _, p := range plugins {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", p.ID, p.Version, p.Status, signedLabel(p.Signer), p.Name)
			}
			return tw.Flush()
		},
	}

	info := &cobra.Command{
		Use:   "info <id>",
		Short: "Show details of an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newPluginManager()
			if err != nil {
				return err
			}
			p, err := mgr.Get(args[0])
			if err != nil {
				return err
			}
			if jsonFlag {
				return writePluginJSON(p)
			}
			bin, _ := mgr.EntrypointPath(p.ID)
			printPlugin(p, bin)
			return nil
		},
	}

	remove := &cobra.Command{
		Use:   "remove <id>",
		Short: "Uninstall a plugin (its snapshots stay in the repository)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newPluginManager()
			if err != nil {
				return err
			}
			if err := mgr.Remove(args[0]); err != nil {
				return err
			}
			if !quietFlag && !jsonFlag {
				fmt.Printf("Removed plugin %s\n", args[0])
			}
			return nil
		},
	}

	cmd.AddCommand(install, upgrade, list, info, remove)
	return cmd
}

func runPluginInstall(cmd *cobra.Command, path string, opts pluginmgr.InstallOptions) error {
	mgr, err := newPluginManager()
	if err != nil {
		return err
	}
	p, err := mgr.Install(cmd.Context(), path, opts)
	if err != nil {
		return err
	}
	if jsonFlag {
		return writePluginJSON(p)
	}
	if !quietFlag {
		verb := "Installed"
		if opts.Upgrade {
			verb = "Upgraded to"
		}
		fmt.Printf("%s %s %s (%s)\n", verb, p.ID, p.Version, signedLabel(p.Signer))
	}
	return nil
}

// newPluginManager builds the plugin manager from the agent configuration.
// Without --config it reads the installed agent's config file, so the CLI
// applies the same trust settings as the agent service.
func newPluginManager() (*pluginmgr.Manager, error) {
	cfg := loadedConfig
	if configFlag == "" {
		if c, err := config.Load(filepath.Join(agent.DefaultConfigDir, "agent-config.yaml")); err == nil && c.Exists {
			cfg = c
		}
	}
	trust, err := pluginpkg.NewTrustPolicy(cfg.PluginTrustedKeys, cfg.PluginAllowUnsigned)
	if err != nil {
		return nil, err
	}
	dir := agent.DefaultPluginStoreDir
	switch {
	case pluginStoreFlag != "":
		dir = pluginStoreFlag
	case cfg.PluginStoreDir != "":
		dir = cfg.PluginStoreDir
	}
	return pluginmgr.New(pluginmgr.Options{Dir: dir, Trust: trust, AgentVersion: Version}), nil
}

func signedLabel(signer string) string {
	if signer == "" {
		return "unsigned"
	}
	for _, k := range pluginpkg.OfficialKeys {
		if k == signer {
			return "official"
		}
	}
	return "third-party"
}

func printPlugin(p *pluginmgr.InstalledPlugin, binary string) {
	fmt.Printf("ID:           %s\n", p.ID)
	fmt.Printf("Name:         %s\n", p.Name)
	fmt.Printf("Version:      %s\n", p.Version)
	fmt.Printf("Status:       %s\n", p.Status)
	if p.Description != "" {
		fmt.Printf("Description:  %s\n", p.Description)
	}
	fmt.Printf("Protocol:     %d\n", p.ProtocolVersion)
	if len(p.Capabilities) > 0 {
		fmt.Printf("Capabilities: %s\n", strings.Join(p.Capabilities, ", "))
	}
	if p.Signer != "" {
		fmt.Printf("Signed by:    %s (%s)\n", p.Signer, signedLabel(p.Signer))
	} else {
		fmt.Printf("Signed by:    unsigned\n")
	}
	fmt.Printf("Installed:    %s\n", p.InstalledAt.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Binary:       %s\n", binary)
}

func writePluginJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// installedPlugins lists the plugins in the agent's store for inventory
// reporting.
func installedPlugins() ([]pluginmgr.InstalledPlugin, error) {
	mgr, err := newPluginManager()
	if err != nil {
		return nil, err
	}
	return mgr.List()
}

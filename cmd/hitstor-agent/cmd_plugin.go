package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hooperits/hitstor/internal/agent"
	"github.com/hooperits/hitstor/internal/config"
	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
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

	cmd.AddCommand(install, upgrade, list, info, remove, newPluginBrowseCmd(), newPluginTestBackupCmd())
	return cmd
}

// openInstalledWorkload launches an installed plugin, marked in use, and
// configures it. The returned close function releases everything.
func openInstalledWorkload(id string, settings map[string]string) (*pluginmgr.InstalledPlugin, *hitstorplugin.WorkloadGRPCClient, func(), error) {
	mgr, err := newPluginManager()
	if err != nil {
		return nil, nil, nil, err
	}
	p, bin, release, err := mgr.Acquire(id)
	if err != nil {
		return nil, nil, nil, err
	}
	c, err := hitstorplugin.OpenWorkload(bin)
	if err != nil {
		release()
		return nil, nil, nil, err
	}
	closeAll := func() {
		c.Close()
		release()
	}
	if len(settings) > 0 {
		if err := c.Configure(context.Background(), settings); err != nil {
			closeAll()
			return nil, nil, nil, err
		}
	}
	return p, c, closeAll, nil
}

func newPluginBrowseCmd() *cobra.Command {
	var settings map[string]string
	cmd := &cobra.Command{
		Use:   "browse <id> [path]",
		Short: "List the objects a plugin can back up",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, c, closeAll, err := openInstalledWorkload(args[0], settings)
			if err != nil {
				return err
			}
			defer closeAll()
			p := ""
			if len(args) == 2 {
				p = args[1]
			}
			entries, err := c.Browse(cmd.Context(), p)
			if err != nil {
				return err
			}
			if jsonFlag {
				return writePluginJSON(entries)
			}
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "PATH\tKIND\tSELECTABLE\tCHILDREN")
			for _, e := range entries {
				fmt.Fprintf(tw, "%s\t%s\t%t\t%t\n", e.Path, e.Kind, e.Selectable, e.HasChildren)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringToStringVar(&settings, "setting", nil, "plugin setting key=value (repeatable)")
	return cmd
}

func newPluginTestBackupCmd() *cobra.Command {
	var (
		settings, options map[string]string
		selection         []string
		kind, outDir      string
	)
	cmd := &cobra.Command{
		Use:   "test-backup <id>",
		Short: "Run a plugin backup into a local directory (diagnostics; nothing is sent to the server)",
		Long: `Run a plugin backup and write each item, with its metadata, under --out.

Nothing is sent to the server and no snapshot is created. Use it to check a
plugin's connection settings and output on this host.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := os.Mkdir(outDir, 0o700); err != nil {
				return fmt.Errorf("--out must be a new directory: %w", err)
			}
			_, c, closeAll, err := openInstalledWorkload(args[0], settings)
			if err != nil {
				return err
			}
			defer closeAll()

			items, err := c.Backup(cmd.Context(), hitstorplugin.BackupRequest{Selection: selection, Kind: kind, Options: options},
				func(itemPath string, data io.Reader) error {
					dst := filepath.Join(outDir, filepath.FromSlash(itemPath))
					if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
						return err
					}
					f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
					if err != nil {
						return err
					}
					if _, err := io.Copy(f, data); err != nil {
						f.Close()
						return err
					}
					return f.Close()
				})
			if err != nil {
				return err
			}
			for _, it := range items {
				if len(it.Metadata) > 0 {
					dst := filepath.Join(outDir, filepath.FromSlash(it.Path)) + hitstorplugin.MetadataSuffix
					if err := os.WriteFile(dst, it.Metadata, 0o600); err != nil {
						return err
					}
				}
			}
			if jsonFlag {
				return writePluginJSON(items)
			}
			for _, it := range items {
				fmt.Printf("%s  %d bytes\n", it.Path, it.Size)
			}
			fmt.Printf("%d item(s) written to %s\n", len(items), outDir)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringSliceVar(&selection, "select", nil, "object path to back up (repeatable)")
	f.StringVar(&kind, "kind", "full", "backup kind")
	f.StringToStringVar(&settings, "setting", nil, "plugin setting key=value (repeatable)")
	f.StringToStringVar(&options, "option", nil, "backup option key=value (repeatable)")
	f.StringVar(&outDir, "out", "", "directory to create and write items into")
	cmd.MarkFlagRequired("select")
	cmd.MarkFlagRequired("out")
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

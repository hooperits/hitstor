// Package bmrcli exposes the bare-metal recovery media commands as a cobra
// command tree.
//
// Both binaries need them. `hitstor bmr` is what the design and the runbooks
// name, while `hitstor-agent bmr` is what actually exists on a protected host:
// the agent package ships hitstor-agent alone, and recovery media can only be
// built on the machine it protects. Sharing one command tree keeps the two
// from drifting.
package bmrcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/hooperits/hitstor/internal/bmr"
	"github.com/spf13/cobra"
)

// Options adapts the command tree to its host binary.
type Options struct {
	// JSON reports whether the binary's global --json flag is set. Read at
	// run time, so it picks up a flag parsed after the command was built.
	JSON func() bool
	// ServerAddr supplies the default server address baked into the recovery
	// hook's prompt. Optional.
	ServerAddr func() string
}

func (o Options) json() bool {
	return o.JSON != nil && o.JSON()
}

func (o Options) serverAddr() string {
	if o.ServerAddr == nil {
		return ""
	}
	return o.ServerAddr()
}

// New builds the `bmr` command tree.
func New(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bmr",
		Short: "Build and inspect bare-metal recovery media",
		Long: `Bare-metal recovery media lets a machine with a blank disk boot, recreate its
disk layout, and pull its data back from HitStor.

These commands run on the protected host itself: the image embeds that host's
kernel, drivers and disk layout, so it cannot be produced from a backup alone.
Building media is a deliberate action, not something a backup does for you —
run it after the host's storage layout changes.

Linux only; HitStor drives Relax-and-Recover (ReaR) underneath.`,
	}
	cmd.AddCommand(newCheckCmd(opts), newGenerateConfigCmd(opts), newBuildMediaCmd(opts))
	return cmd
}

// configFlags are the media-shaping flags shared by generate-config and
// build-media.
type configFlags struct {
	outputDir   string
	agentBinary string
	hookScript  string
	configPath  string
	serverAddr  string
	agentName   string
	migration   bool
	copyAsIs    []string
}

func (f *configFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.outputDir, "output-dir", bmr.DefaultOutputDir,
		"directory ReaR writes the finished ISO into")
	cmd.Flags().StringVar(&f.agentBinary, "agent-binary", bmr.DefaultAgentBinary,
		"hitstor-agent binary to embed in the rescue image")
	cmd.Flags().StringVar(&f.hookScript, "hook-script", bmr.DefaultHookScript,
		"path to write the ReaR EXTERNAL_RESTORE hook to")
	cmd.Flags().StringVar(&f.configPath, "config-path", bmr.DefaultReaRConfig,
		"path to write the generated ReaR configuration to")
	cmd.Flags().StringVar(&f.serverAddr, "server", "",
		"HitStor server address to prefill in the recovery prompt (host:port)")
	cmd.Flags().StringVar(&f.agentName, "agent", "",
		"registered agent name to prefill in the recovery prompt")
	cmd.Flags().BoolVar(&f.migration, "migration-mode", false,
		"allow recovery onto dissimilar hardware or a virtual machine (ReaR MIGRATION_MODE)")
	cmd.Flags().StringArrayVar(&f.copyAsIs, "copy", nil,
		"additional absolute path to embed in the rescue image (repeatable)")
}

func (f *configFlags) config(opts Options) bmr.ConfigOptions {
	server := f.serverAddr
	if server == "" {
		server = opts.serverAddr()
	}
	return bmr.ConfigOptions{
		OutputDir:     f.outputDir,
		AgentBinary:   f.agentBinary,
		HookScript:    f.hookScript,
		ServerAddr:    server,
		AgentName:     f.agentName,
		MigrationMode: f.migration,
		ExtraCopyAsIs: f.copyAsIs,
	}
}

func newCheckCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Report whether this host can build recovery media",
		Long: `Check reports everything standing between this host and a bootable rescue
image: whether ReaR is installed and which version, which supporting tools are
present, and whether the command has the privileges ReaR needs.

It reports every gap at once rather than stopping at the first, and exits
non-zero when the host is not ready, so it can gate a provisioning script.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			readiness := bmr.Check(cmd.Context())
			if opts.json() {
				return writeJSON(cmd.OutOrStdout(), readiness)
			}
			printReadiness(cmd.OutOrStdout(), readiness)
			if !readiness.Ready {
				// The blockers are already printed in full above; this only
				// has to make the exit status non-zero for a caller
				// scripting around it.
				return errNotReady
			}
			return nil
		},
	}
}

// errNotReady makes an unready host a non-zero exit without repeating the
// diagnosis the command has already printed in full.
var errNotReady = errors.New("host is not ready to build bare-metal recovery media (see above)")

func newGenerateConfigCmd(opts Options) *cobra.Command {
	var flags configFlags
	var stdout bool

	cmd := &cobra.Command{
		Use:   "generate-config",
		Short: "Write the ReaR configuration that hands recovery to HitStor",
		Long: `Writes /etc/rear/local.conf and the EXTERNAL_RESTORE hook the rescue system
runs once it has recreated the disk layout.

No credentials are written: the recovery token and, where needed, the
repository password are prompted for at recovery time, so the media can be
stored without becoming a way into the repository.

build-media regenerates both files, so running this separately is only needed
to review or hand-edit them first.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := flags.config(opts)

			if stdout {
				rendered, err := bmr.RenderConfig(cfg)
				if err != nil {
					return err
				}
				_, err = io.WriteString(cmd.OutOrStdout(), rendered)
				return err
			}

			if err := bmr.WriteConfigFiles(flags.configPath, cfg); err != nil {
				return err
			}
			if opts.json() {
				return writeJSON(cmd.OutOrStdout(), map[string]string{
					"config_path": flags.configPath,
					"hook_script": cfg.HookScript,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote ReaR configuration to %s\n", flags.configPath)
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote recovery hook to      %s\n", cfg.HookScript)
			return nil
		},
	}
	flags.register(cmd)
	cmd.Flags().BoolVar(&stdout, "stdout", false, "print the configuration instead of writing it")
	return cmd
}

func newBuildMediaCmd(opts Options) *cobra.Command {
	var flags configFlags
	var force bool

	cmd := &cobra.Command{
		Use:   "build-media",
		Short: "Build a bootable recovery ISO for this host",
		Long: `Runs ReaR to produce a bootable rescue image for this host, after writing the
configuration that points its recovery run back at HitStor.

The build takes several minutes and reads every disk on the machine, so it is
deliberately manual. Rebuild it whenever the host's disk layout changes: a
stale image recreates the old layout.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			asJSON := opts.json()

			media, err := bmr.BuildMedia(cmd.Context(), bmr.BuildOptions{
				Config:     flags.config(opts),
				ConfigPath: flags.configPath,
				SkipCheck:  force,
				OnProgress: func(line string) {
					if !asJSON {
						fmt.Fprintf(out, "  %s\n", line)
					}
				},
			})
			if err != nil {
				var notReady *bmr.NotReadyError
				if errors.As(err, &notReady) && !asJSON {
					printReadiness(out, notReady.Readiness)
					fmt.Fprintln(out, "\nFix the blockers above, or pass --force to build anyway.")
					return errNotReady
				}
				return err
			}

			if asJSON {
				return writeJSON(out, media)
			}
			fmt.Fprintf(out, "\nRecovery media built: %s (%s)\n",
				media.ISOPath, humanBytes(media.SizeBytes))
			fmt.Fprintln(out, "Store it somewhere that survives losing this host.")
			return nil
		},
	}
	flags.register(cmd)
	cmd.Flags().BoolVar(&force, "force", false,
		"build even when the readiness check reports blockers")
	return cmd
}

func printReadiness(w io.Writer, r bmr.Readiness) {
	if r.Ready {
		fmt.Fprintln(w, "Ready to build bare-metal recovery media.")
	} else {
		fmt.Fprintln(w, "NOT ready to build bare-metal recovery media.")
	}
	fmt.Fprintf(w, "\nPlatform: %s\n", r.Platform)
	if r.ReaRInstalled {
		fmt.Fprintf(w, "ReaR:     %s (%s)\n", r.ReaRVersion, r.ReaRPath)
	} else {
		fmt.Fprintln(w, "ReaR:     not installed")
	}
	fmt.Fprintf(w, "Root:     %t\n", r.Root)

	if len(r.Tools) > 0 {
		fmt.Fprintln(w, "\nTools:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, t := range r.Tools {
			mark, kind := "missing", "optional"
			if t.Found {
				mark = t.Path
			}
			if t.Required {
				kind = "required"
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", t.Name, kind, mark, t.Purpose)
		}
		tw.Flush()
	}

	printList(w, "Blockers", r.Blockers)
	printList(w, "Warnings", r.Warnings)
}

func printList(w io.Writer, heading string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s:\n", heading)
	for _, item := range items {
		fmt.Fprintf(w, "  - %s\n", item)
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

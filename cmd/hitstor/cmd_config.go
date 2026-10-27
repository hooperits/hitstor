package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Aliases: []string{"cfg"},
		Short:   "Manage HitStor configuration",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	cmd.AddCommand(newConfigShowCmd())
	cmd.AddCommand(newConfigSetCmd())
	cmd.AddCommand(newConfigUnsetCmd())
	cmd.AddCommand(newConfigAddCmd())
	cmd.AddCommand(newConfigRemoveCmd())
	cmd.AddCommand(newConfigInitCmd())
	cmd.AddCommand(newConfigEditCmd())
	cmd.AddCommand(newConfigPathCmd())
	cmd.AddCommand(newConfigRepositoriesCmd())

	return cmd
}

// configPath returns the config file path from --config flag or the effective default.
// We give strong preference to /etc/hitstor/config.yaml (the one managed by the
// dashboard and used by the service) whenever it exists on disk. This makes
// everyday CLI usage on the server box see the real multi-repo configuration.
func configPath() string {
	if configFlag != "" {
		return configFlag
	}
	// Server-friendly default: if we can actually read the system config, use it.
	// This is the most common need for admins on the server box.
	if f, err := os.Open(config.SystemConfigPath); err == nil {
		f.Close()
		return config.SystemConfigPath
	}
	return config.EffectiveDefaultPath()
}

// cobraFlagChecker adapts cobra's flag set to config.FlagChecker.
type cobraFlagChecker struct {
	cmd *cobra.Command
}

func (c cobraFlagChecker) Changed(name string) bool {
	f := c.cmd.Root().Flags().Lookup(name)
	if f == nil {
		f = c.cmd.Root().PersistentFlags().Lookup(name)
	}
	if f == nil {
		return false
	}
	return f.Changed
}

// --- config show ---

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display all configuration values and their sources",
		Args:  cobra.NoArgs,
		RunE:  runConfigShow,
	}
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	path := configPath()
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	flags := cobraFlagChecker{cmd: cmd}
	envLookup := func(key string) (string, bool) {
		val, ok := os.LookupEnv(key)
		return val, ok
	}

	resolved := config.ResolveAll(cfg, flags, envLookup)

	if jsonFlag {
		return configShowJSON(cfg, resolved)
	}
	return configShowText(cfg, resolved)
}

func configShowJSON(cfg config.Config, resolved []config.ResolvedKey) error {
	type jsonKey struct {
		Key    string      `json:"key"`
		Value  interface{} `json:"value"`
		Source string      `json:"source"`
	}

	keys := make([]jsonKey, 0, len(resolved))
	for _, rk := range resolved {
		var val interface{}
		switch rk.Def.Type {
		case config.List:
			if rk.Value == "" {
				val = []string{}
			} else {
				val = strings.Split(rk.Value, ", ")
			}
		case config.Int:
			// Parse int for JSON output.
			var n int
			fmt.Sscanf(rk.Value, "%d", &n)
			val = n
		default:
			displayVal := rk.Value
			if rk.Def.Sensitive {
				displayVal = config.MaskValue(displayVal)
			}
			val = displayVal
		}

		keys = append(keys, jsonKey{
			Key:    rk.Def.Name,
			Value:  val,
			Source: rk.Source,
		})
	}

	output := map[string]interface{}{
		"config_file":        cfg.Path,
		"config_file_exists": cfg.Exists,
		"keys":               keys,
		"repositories":       cfg.Repositories, // modern multi-repo (may contain encrypted fields)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(output)
}

func configShowText(cfg config.Config, resolved []config.ResolvedKey) error {
	// Always show which config file is being used (critical for server vs desktop)
	path := cfg.Path
	if path == "" {
		path = configPath()
	}
	fmt.Fprintf(os.Stdout, "Config file: %s", path)
	if path == config.SystemConfigPath {
		fmt.Print("   (system default)")
	} else if strings.Contains(path, "/.config/hitstor/") {
		fmt.Print("   (user config)")
	}
	fmt.Println()

	// Modern multi-repo view FIRST when present (this is the real configuration now)
	if len(cfg.Repositories) > 0 {
		fmt.Fprintln(os.Stdout)
		fmt.Printf("Configured repositories (%d):\n", len(cfg.Repositories))
		for i, rc := range cfg.Repositories {
			fmt.Printf("  [%d] %s\n", i, rc.Name)
			fmt.Printf("      Type:           %s\n", rc.Type)
			if rc.Type == "local" && rc.Path != "" {
				fmt.Printf("      Path:           %s\n", rc.Path)
			} else if rc.Type == "s3" {
				fmt.Printf("      Endpoint:       %s\n", rc.Endpoint)
				fmt.Printf("      Bucket:         %s\n", rc.Bucket)
				if rc.Region != "" {
					fmt.Printf("      Region:         %s\n", rc.Region)
				}
			}

			if rc.PasswordFile != "" {
				status := "[not found]"
				if _, err := os.Stat(rc.PasswordFile); err == nil {
					status = "[exists, readable]"
				}
				fmt.Printf("      Password file:  %s %s\n", rc.PasswordFile, status)
			}

			if rc.AccessKeyEncrypted != "" || rc.SecretKeyEncrypted != "" {
				fmt.Println("      Credentials:    stored encrypted")
			} else if rc.Type == "s3" {
				fmt.Println("      Credentials:    (not set)")
			}
		}
	}

	// Server-relevant settings that actually came from the config file
	fmt.Fprintln(os.Stdout)
	fmt.Println("Server settings (from this file):")
	serverKeys := []string{
		"serve_port", "serve_advertise_addr", "serve_tls_cert", "serve_tls_key",
		"serve_dashboard_port", "serve_dashboard_password", "serve_dashboard_language",
	}
	shownServer := false
	for _, rk := range resolved {
		for _, k := range serverKeys {
			if rk.Def.Name == k && rk.Source != "default" {
				val := rk.Value
				if rk.Def.Sensitive && val != "" {
					val = config.MaskValue(val)
				}
				fmt.Printf("  %-24s %s\n", rk.Def.Name+":", val)
				shownServer = true
				break
			}
		}
	}
	if !shownServer {
		fmt.Println("  (none customized in this file)")
	}

	// Legacy single-repo / flat keys - only show the interesting ones (non-default)
	fmt.Fprintln(os.Stdout)
	fmt.Println("Legacy single-repo keys (for backward compatibility):")
	legacyShown := false
	legacySkip := map[string]bool{
		"repo": true, "password_file": true,
		"serve_port": true, "serve_advertise_addr": true, "serve_tls_cert": true,
		"serve_tls_key": true, "serve_dashboard_port": true, "serve_dashboard_password": true,
		"serve_dashboard_language": true,
	}
	for _, rk := range resolved {
		if legacySkip[rk.Def.Name] {
			continue
		}
		if rk.Source != "default" {
			val := rk.Value
			if rk.Def.Sensitive && val != "" {
				val = config.MaskValue(val)
			}
			if val == "" {
				val = "(not set)"
			}
			fmt.Printf("  %-24s %-20s %s\n", rk.Def.Name, val, rk.Source)
			legacyShown = true
		}
	}
	if !legacyShown {
		fmt.Println("  (all at defaults)")
	}

	// Actionable footer
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Use `hitstor repo` (list/create/delete) or the dashboard Storage page to manage repositories.")
	fmt.Fprintln(os.Stdout, "Use `--repo-name <name>` for ad-hoc commands (backup, snapshots, etc.).")

	return nil
}

// --- config set ---

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value (legacy flat keys; use `repo` subcommand for modern repositories list)",
		Args:  cobra.ExactArgs(2),
		RunE:  runConfigSet,
	}
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key, value := args[0], args[1]
	path := configPath()

	old, err := config.Set(path, key, value)
	if err != nil {
		return err
	}

	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(map[string]string{
			"key":      key,
			"value":    value,
			"previous": old,
		})
	}

	color := ui.NewColorWriter(os.Stdout, quietFlag)
	oldDisplay := old
	if oldDisplay == "" {
		oldDisplay = "not set"
	}
	fmt.Fprintln(os.Stdout, color.Green(fmt.Sprintf("Set %s = %s (was: %s)", key, value, oldDisplay)))
	return nil
}

// --- config unset ---

func newConfigUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unset <key>",
		Short: "Remove a configuration value (revert to default)",
		Args:  cobra.ExactArgs(1),
		RunE:  runConfigUnset,
	}
}

func runConfigUnset(cmd *cobra.Command, args []string) error {
	key := args[0]
	path := configPath()

	old, wasSet, err := config.Unset(path, key)
	if err != nil {
		return err
	}

	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(map[string]interface{}{
			"key":      key,
			"previous": old,
			"unset":    wasSet,
		})
	}

	color := ui.NewColorWriter(os.Stdout, quietFlag)
	if wasSet {
		fmt.Fprintln(os.Stdout, color.Green(fmt.Sprintf("Unset %s (was: %s)", key, old)))
	} else {
		fmt.Fprintf(os.Stdout, "Key %s is not set in config file\n", key)
	}
	return nil
}

// --- config add ---

func newConfigAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <key> <value>",
		Short: "Append a value to a list configuration key",
		Args:  cobra.ExactArgs(2),
		RunE:  runConfigAdd,
	}
}

func runConfigAdd(cmd *cobra.Command, args []string) error {
	key, value := args[0], args[1]
	path := configPath()

	newList, err := config.AddToList(path, key, value)
	if err != nil {
		return err
	}

	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(map[string]interface{}{
			"key":   key,
			"added": value,
			"value": newList,
		})
	}

	color := ui.NewColorWriter(os.Stdout, quietFlag)
	fmt.Fprintln(os.Stdout, color.Green(fmt.Sprintf("Added %q to %s (now: %s)", value, key, strings.Join(newList, ", "))))
	return nil
}

// --- config remove ---

func newConfigRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <key> <value>",
		Short: "Remove a value from a list configuration key",
		Args:  cobra.ExactArgs(2),
		RunE:  runConfigRemove,
	}
}

func runConfigRemove(cmd *cobra.Command, args []string) error {
	key, value := args[0], args[1]
	path := configPath()

	newList, found, err := config.RemoveFromList(path, key, value)
	if err != nil {
		return err
	}

	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(map[string]interface{}{
			"key":     key,
			"removed": value,
			"value":   newList,
		})
	}

	color := ui.NewColorWriter(os.Stdout, quietFlag)
	if found {
		fmt.Fprintln(os.Stdout, color.Green(fmt.Sprintf("Removed %q from %s (now: %s)", value, key, strings.Join(newList, ", "))))
	} else {
		fmt.Fprintf(os.Stdout, "Value %q not found in %s\n", value, key)
	}
	return nil
}

// --- config init ---

func newConfigInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Interactive configuration setup wizard",
		Args:  cobra.NoArgs,
		RunE:  runConfigInit,
	}
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	if !isTerminalStdin() {
		return fmt.Errorf("config init requires an interactive terminal\nHint: Use \"hitstor config set <key> <value>\" for non-interactive configuration")
	}

	path := configPath()
	existing, err := config.Load(path)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(os.Stdin)
	color := ui.NewColorWriter(os.Stdout, quietFlag || jsonFlag)

	// Print banner.
	printBanner()
	fmt.Fprintln(os.Stdout, color.Green("Welcome to HitStor configuration setup!"))
	fmt.Fprintln(os.Stdout)

	// Warn if config exists.
	if existing.Exists {
		fmt.Fprintf(os.Stdout, "%s\n", color.Yellow("Config file already exists at "+path))
		overwrite := promptLine(scanner, "Overwrite? (y/N)", "N")
		if !strings.EqualFold(overwrite, "y") {
			fmt.Fprintln(os.Stdout, "Cancelled.")
			return nil
		}
		fmt.Fprintln(os.Stdout)
	}

	// Ask backend type.
	defaultBackend := "local"
	if existing.Exists && strings.HasPrefix(existing.Repo, "s3:") {
		defaultBackend = "s3"
	}
	backendType := promptLine(scanner, "Backend type (local/s3)", defaultBackend)
	if backendType != "local" && backendType != "s3" {
		backendType = defaultBackend
	}
	fmt.Fprintln(os.Stdout)

	var cfg config.Config
	cfg.Path = path

	if backendType == "s3" {
		// S3 flow.
		defaultRepo := ""
		if existing.Exists {
			defaultRepo = existing.Repo
		}
		cfg.Repo = promptLine(scanner, "Repository (s3:endpoint/bucket)", defaultRepo)

		defaultAK := ""
		if existing.Exists {
			defaultAK = existing.S3AccessKey
		}
		cfg.S3AccessKey = promptLine(scanner, "S3 access key", defaultAK)

		defaultSK := ""
		if existing.Exists {
			defaultSK = existing.S3SecretKey
		}
		cfg.S3SecretKey = promptLine(scanner, "S3 secret key", defaultSK)

		defaultRegion := "us-east-1"
		if existing.Exists && existing.S3Region != "" {
			defaultRegion = existing.S3Region
		}
		cfg.S3Region = promptLine(scanner, "S3 region", defaultRegion)
	} else {
		// Local flow.
		defaultRepo := ""
		if existing.Exists {
			defaultRepo = existing.Repo
		}
		cfg.Repo = promptLine(scanner, "Repository path", defaultRepo)
	}

	// Common prompts.
	defaultPWFile := ""
	if existing.Exists {
		defaultPWFile = existing.PasswordFile
	}
	cfg.PasswordFile = promptLine(scanner, "Password file (optional)", defaultPWFile)

	defaultParallel := "0"
	if existing.Exists && existing.Parallel > 0 {
		defaultParallel = fmt.Sprintf("%d", existing.Parallel)
	}
	parallelStr := promptLine(scanner, "Parallel workers (0 = auto)", defaultParallel)
	fmt.Sscanf(parallelStr, "%d", &cfg.Parallel)

	defaultExclude := ""
	if existing.Exists && len(existing.Exclude) > 0 {
		defaultExclude = strings.Join(existing.Exclude, ",")
	}
	excludeStr := promptLine(scanner, "Exclude patterns (comma-separated, optional)", defaultExclude)
	if excludeStr != "" {
		parts := strings.Split(excludeStr, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		cfg.Exclude = parts
	}

	// Save atomically.
	if err := config.Save(cfg); err != nil {
		return err
	}

	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, color.Green("Configuration saved to "+path))
	return nil
}

// promptLine displays a prompt with an optional default and reads one line.
func promptLine(scanner *bufio.Scanner, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Fprintf(os.Stdout, "%s [%s]: ", label, defaultVal)
	} else {
		fmt.Fprintf(os.Stdout, "%s: ", label)
	}

	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			return defaultVal
		}
		return input
	}
	return defaultVal
}

// isTerminalStdin checks if stdin is connected to an interactive terminal.
func isTerminalStdin() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// --- config edit ---

func newConfigEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open configuration file in your editor",
		Args:  cobra.NoArgs,
		RunE:  runConfigEdit,
	}
}

func runConfigEdit(cmd *cobra.Command, args []string) error {
	path := configPath()
	color := ui.NewColorWriter(os.Stdout, quietFlag || jsonFlag)

	// Create template if file doesn't exist.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		template := "# HitStor configuration\n# See: hitstor config show\n\n# repo: /path/to/repository\n# password_file: /path/to/password\n# s3_access_key: \"\"\n# s3_secret_key: \"\"\n# s3_region: us-east-1\n# exclude:\n#   - \"*.tmp\"\n# parallel: 0\n"
		dir := path[:strings.LastIndex(path, "/")]
		os.MkdirAll(dir, 0700)
		if err := os.WriteFile(path, []byte(template), 0600); err != nil {
			return fmt.Errorf("config: create template: %w", err)
		}
	}

	// Resolve editor.
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	fmt.Fprintf(os.Stdout, "Opening %s in %s...\n", path, editor)

	for {
		c := exec.Command(editor, path)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("config: editor failed: %w", err)
		}

		// Validate YAML after edit.
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("config: read after edit: %w", err)
		}

		var check config.Config
		if err := yaml.Unmarshal(data, &check); err != nil {
			fmt.Fprintf(os.Stdout, "%s\n", color.Yellow("Warning: config file has invalid YAML syntax"))
			if isTerminalStdin() {
				scanner := bufio.NewScanner(os.Stdin)
				answer := promptLine(scanner, "Re-open editor? (y/N)", "N")
				if strings.EqualFold(answer, "y") {
					continue
				}
			}
			fmt.Fprintln(os.Stdout, "Kept changes with invalid YAML.")
			return nil
		}

		fmt.Fprintln(os.Stdout, color.Green("Config file validated successfully."))
		return nil
	}
}

// --- config path ---

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Show configuration file path",
		Args:  cobra.NoArgs,
		RunE:  runConfigPath,
	}
}

func runConfigPath(cmd *cobra.Command, args []string) error {
	path := configPath()
	_, exists := os.Stat(path)

	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(map[string]interface{}{
			"path":   path,
			"exists": exists == nil,
		})
	}

	fmt.Fprintln(os.Stdout, path)

	// Helpful note for server admins (the most common source of confusion).
	if path == config.SystemConfigPath && !jsonFlag {
		fmt.Fprintln(os.Stdout, "(using system config /etc/hitstor/config.yaml — recommended on servers)")
	} else if strings.HasPrefix(path, "/home/") && strings.Contains(path, "/.config/hitstor/") && !jsonFlag {
		fmt.Fprintln(os.Stdout, "(using per-user config; on servers prefer /etc/hitstor/config.yaml or pass --config)")
	}

	return nil
}

// --- config repositories (modern multi-repo support) ---

func newConfigRepositoriesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repositories",
		Short: "Manage or view the modern repositories list (alias for top-level `repo`)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List repositories (same as `hitstor repo list`)",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Reuse the repo list logic
			return runRepoList(cmd, args)
		},
	})
	// Could add more like add/remove in future, but delegate to `repo` for now
	return cmd
}

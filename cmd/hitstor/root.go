package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/hooperits/hitstor/internal/backend"
	"github.com/hooperits/hitstor/internal/backend/local"
	s3backend "github.com/hooperits/hitstor/internal/backend/s3"
	"github.com/hooperits/hitstor/internal/bmr/bmrcli"
	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/logstore"
	hitstorplugin "github.com/hooperits/hitstor/internal/plugin"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/server"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	repoFlag         string
	repoNameFlag     string // for multi-repo selection by name
	passwordFlag     string
	passwordFileFlag string
	verboseCount     int
	quietFlag        bool
	jsonFlag         bool
	configFlag       string

	// S3 flags
	s3AccessKeyFlag string
	s3SecretKeyFlag string
	s3RegionFlag    string

	// Parallel workers flag (0 = auto-detect).
	parallelFlag int

	// Exclude patterns from config file (merged with --exclude flags).
	configExcludes []string

	// Parallel workers from config file.
	configParallel int

	// breakLockFlag removes stale repository locks before mutating operations.
	breakLockFlag bool

	// loadedConfig holds the parsed config file for use by config subcommands.
	loadedConfig config.Config
)

const banner = ` _   _ _ _   ____  _
| | | (_) |_/ ___|| |_ ___  _ __
| |_| | | __\___ \| __/ _ \| '__|
|  _  | | |_ ___) | || (_) | |
|_| |_|_|\__|____/ \__\___/|_|`

var rootCmd = &cobra.Command{
	Use:           "hitstor",
	Short:         "Encrypted, deduplicated backup tool",
	Long:          "HitStor is an encrypted, deduplicated backup tool for Linux filesystems with local and S3-compatible storage backends.",
	SilenceUsage:  true,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		printBanner()
		cmd.Help()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Load YAML config.
		loadConfig()

		// Environment variable overrides (after config, before flag check).
		if env := os.Getenv("HITSTOR_REPO"); env != "" && repoFlag == "" {
			repoFlag = env
		}
		if env := os.Getenv("HITSTOR_PASSWORD"); env != "" && passwordFlag == "" {
			passwordFlag = env
		}

		// Setup structured logging.
		setupLogging()
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&repoFlag, "repo", "r", "", "repository location (path or s3:endpoint/bucket) (legacy single-repo)")
	rootCmd.PersistentFlags().StringVar(&repoNameFlag, "repo-name", "", "select repository by name from multi-repo config")
	rootCmd.PersistentFlags().StringVar(&passwordFlag, "password", "", "repository password")
	rootCmd.PersistentFlags().StringVar(&passwordFileFlag, "password-file", "", "read password from file")
	rootCmd.PersistentFlags().CountVarP(&verboseCount, "verbose", "v", "increase log verbosity (repeat for more: -vv)")
	rootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "suppress non-error output")
	rootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "output in JSON format")
	rootCmd.PersistentFlags().StringVar(&configFlag, "config", "", "config file path (default: ~/.config/hitstor/config.yaml)")
	rootCmd.PersistentFlags().IntVar(&parallelFlag, "parallel", 0, "number of parallel workers (0 = auto)")
	rootCmd.PersistentFlags().StringVar(&s3AccessKeyFlag, "s3-access-key", "", "S3 access key")
	rootCmd.PersistentFlags().StringVar(&s3SecretKeyFlag, "s3-secret-key", "", "S3 secret key")
	rootCmd.PersistentFlags().StringVar(&s3RegionFlag, "s3-region", "us-east-1", "S3 region")
	rootCmd.PersistentFlags().BoolVar(&breakLockFlag, "break-lock", false, "remove stale repository locks before the operation")

	// Register dynamic completion for --repo-name
	rootCmd.RegisterFlagCompletionFunc("repo-name", completeRepoNames)

	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newBackupCmd())
	rootCmd.AddCommand(newRestoreCmd())
	rootCmd.AddCommand(newVerifyRestoreCmd())
	rootCmd.AddCommand(newSnapshotsCmd())
	rootCmd.AddCommand(newForgetCmd())
	rootCmd.AddCommand(newCheckCmd())
	rootCmd.AddCommand(newPruneCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newDuCmd())
	rootCmd.AddCommand(newDiffCmd())
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newServeCmd())
	rootCmd.AddCommand(newRepoCmd())
	rootCmd.AddCommand(newJobsCmd())
	rootCmd.AddCommand(newAgentsCmd())
	rootCmd.AddCommand(bmrcli.New(bmrcli.Options{
		JSON: func() bool { return jsonFlag },
	}))
}

// printBanner displays the ASCII art banner if output is an interactive terminal
// and neither --quiet nor --json is active.
func printBanner() {
	if quietFlag || jsonFlag {
		return
	}
	if !isTerminalStdout() {
		return
	}
	fmt.Fprintf(os.Stdout, "%s   v%s\n\n", banner, Version)
}

// isTerminalStdout checks if stdout is connected to an interactive terminal.
func isTerminalStdout() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// setupLogging configures the global slog logger based on flags.
func setupLogging() {
	var level slog.Level
	switch {
	case quietFlag:
		// Discard all log output.
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		return
	case verboseCount >= 2:
		level = slog.LevelDebug
	case verboseCount == 1:
		level = slog.LevelInfo
	default:
		level = slog.LevelWarn
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if jsonFlag {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))
}

// initLogStore enables in-memory and on-disk log retention for View Logs.
func initLogStore(cfg config.Config, source string) {
	if quietFlag {
		return
	}
	if err := logstore.InitFromConfig(config.LogStoreConfig(cfg)); err != nil {
		slog.Warn("logstore init failed", "error", err)
		return
	}
	level := slog.LevelInfo
	switch {
	case verboseCount >= 2:
		level = slog.LevelDebug
	case verboseCount == 1:
		level = slog.LevelInfo
	case quietFlag:
		level = slog.LevelWarn
	}
	opts := &slog.HandlerOptions{Level: level}
	var base slog.Handler
	if jsonFlag {
		base = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		base = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(logstore.NewTeeHandler(base, source)))
}

// loadConfig loads the YAML config file and applies values as defaults.
// CLI flags take precedence over config values.
func loadConfig() {
	cfg, err := config.Load(configFlag)
	if err != nil {
		return
	}
	loadedConfig = cfg

	if !cfg.Exists {
		return
	}

	// Apply config as defaults (CLI flags override).
	if repoFlag == "" && cfg.Repo != "" {
		repoFlag = cfg.Repo
	}
	if s3AccessKeyFlag == "" && cfg.S3AccessKey != "" {
		s3AccessKeyFlag = cfg.S3AccessKey
	}
	if s3SecretKeyFlag == "" && cfg.S3SecretKey != "" {
		s3SecretKeyFlag = cfg.S3SecretKey
	}
	if s3RegionFlag == "us-east-1" && cfg.S3Region != "" {
		s3RegionFlag = cfg.S3Region
	}
	configExcludes = cfg.Exclude
	configParallel = cfg.Parallel
}

// getPassword resolves the repository password from flags.
func getPassword() (string, error) {
	if passwordFlag != "" {
		return passwordFlag, nil
	}
	if passwordFileFlag != "" {
		data, err := os.ReadFile(passwordFileFlag)
		if err != nil {
			return "", fmt.Errorf("reading password file: %w", err)
		}
		return string(data), nil
	}
	return "", fmt.Errorf("password required: use --password or --password-file")
}

// openBackend creates a backend from the --repo flag value.
// Local paths are used directly; s3:endpoint/bucket creates an S3 backend;
// plugin:<name>[/<uri>] launches a plugin backend subprocess.
func openBackend() (backend.Backend, error) {
	if repoFlag == "" {
		return nil, fmt.Errorf("--repo flag is required")
	}

	if strings.HasPrefix(repoFlag, "plugin:") {
		return openPluginBackend()
	}

	if strings.HasPrefix(repoFlag, "s3:") {
		// Parse s3:endpoint/bucket format.
		rest := strings.TrimPrefix(repoFlag, "s3:")
		// Find the last '/' to split endpoint from bucket.
		idx := strings.LastIndex(rest, "/")
		if idx < 0 {
			return nil, fmt.Errorf("invalid S3 repo format: expected s3:endpoint/bucket (e.g., s3:http://localhost:9000/mybucket)")
		}
		endpoint := rest[:idx]
		bucket := rest[idx+1:]
		if endpoint == "" || bucket == "" {
			return nil, fmt.Errorf("invalid S3 repo format: endpoint and bucket must not be empty")
		}

		// Auto-prepend http:// if no scheme is specified.
		if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			endpoint = "http://" + endpoint
		}

		if s3AccessKeyFlag == "" || s3SecretKeyFlag == "" {
			return nil, fmt.Errorf("S3 backend requires --s3-access-key and --s3-secret-key flags")
		}

		return s3backend.New(endpoint, bucket, s3AccessKeyFlag, s3SecretKeyFlag, s3RegionFlag)
	}

	return local.NewWithOptions(repoFlag, local.Options{SyncWrites: syncWritesEnabled()})
}

// syncWritesEnabled reports whether local writes should be fsynced after rename.
func syncWritesEnabled() bool {
	return loadedConfig.SyncWrites || os.Getenv("HITSTOR_SYNC_WRITES") == "1"
}

// applyRepositorySettings applies config-driven runtime options to an opened repository.
func applyRepositorySettings(repo *repository.Repository, be backend.Backend) {
	if loadedConfig.MinFreeReserveBytes > 0 {
		repo.SetOptions(repository.RepoOptions{
			MinFreeReserveBytes: loadedConfig.MinFreeReserveBytes,
		})
	}
	if lb, ok := be.(*local.Local); ok {
		lb.SetSyncWrites(syncWritesEnabled())
	}
}

// resolveRepoByName looks up a RepoConfig by name and returns effective legacy-style values
// for backward compat with the rest of the CLI. For S3 it returns the constructed "s3:..." string
// but S3 credentials must still be provided via flags or legacy config (full encrypted support
// for named S3 is planned).
func resolveRepoByName(name string) (effectiveRepo string, effectivePasswordFile string, err error) {
	for _, rc := range loadedConfig.Repositories {
		if rc.Name == name {
			if rc.Type == "local" {
				return rc.Path, rc.PasswordFile, nil
			}
			if rc.Type == "s3" {
				// Construct legacy s3: form. Caller must provide creds or we will fail later.
				ep := rc.Endpoint
				if !strings.HasPrefix(ep, "http") {
					ep = "http://" + ep
				}
				return fmt.Sprintf("s3:%s/%s", strings.TrimPrefix(ep, "http://"), rc.Bucket), rc.PasswordFile, nil
			}
			return "", "", fmt.Errorf("unsupported repo type %q for --repo-name", rc.Type)
		}
	}
	return "", "", fmt.Errorf("repository %q not found in config", name)
}

// openRepository opens the repository from global flags and applies storage settings.
func openRepository() (*repository.Repository, backend.Backend, error) {
	// Support modern --repo-name (multi-repo) by resolving to legacy effective values.
	if repoNameFlag != "" {
		effRepo, effPwFile, err := resolveRepoByName(repoNameFlag)
		if err != nil {
			return nil, nil, err
		}
		if repoFlag == "" {
			repoFlag = effRepo
		}
		if passwordFileFlag == "" && effPwFile != "" {
			passwordFileFlag = effPwFile
		}
	}

	password, err := getPassword()
	if err != nil {
		return nil, nil, err
	}

	be, err := openBackend()
	if err != nil {
		return nil, nil, err
	}

	repo, err := repository.Open(be, password)
	if err != nil {
		be.Close()
		return nil, nil, fmt.Errorf("open repository: %w", err)
	}

	applyRepositorySettings(repo, be)
	return repo, be, nil
}

// openPluginBackend launches a plugin backend from a "plugin:<name>[/<uri>]" repo flag.
func openPluginBackend() (backend.Backend, error) {
	rest := strings.TrimPrefix(repoFlag, "plugin:")

	var name, uri string
	if idx := strings.Index(rest, "/"); idx >= 0 {
		name = rest[:idx]
		uri = rest[idx+1:]
	} else {
		name = rest
	}

	if name == "" {
		return nil, fmt.Errorf("invalid plugin repo format: expected plugin:<name>[/<uri>]")
	}

	settings := make(map[string]string)
	if uri != "" {
		settings["uri"] = uri
	}

	return hitstorplugin.OpenStorageBackend(name, loadedConfig.PluginDir, settings)
}

// completeSnapshotIDs provides dynamic shell completion for snapshot IDs.
// It opens the repository using current flags and returns 8-char ID prefixes.
func completeSnapshotIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	repo, be, err := openRepository()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer be.Close()

	snapIDs, err := repo.ListSnapshots()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, id := range snapIDs {
		short := id.String()[:8]
		if strings.HasPrefix(short, toComplete) {
			completions = append(completions, short)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeRepoNames provides shell completion for --repo-name from the multi-repo config.
func completeRepoNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// loadedConfig is populated in PersistentPreRunE
	var names []string
	for _, r := range loadedConfig.Repositories {
		if strings.HasPrefix(r.Name, toComplete) {
			names = append(names, r.Name)
		}
	}
	if len(names) == 0 {
		// fallback: at least suggest names even if config not fully loaded yet
		for _, r := range loadedConfig.Repositories {
			names = append(names, r.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeAgentNames provides shell completion for agent names from the registry of the selected repo.
func completeAgentNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Try to open current repo context
	repo, be, err := openRepository()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer be.Close()

	basePath, err := backendBasePath(be)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	registry, err := server.LoadAgentRegistry(repo.Key().EncryptKey, basePath)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	agents := registry.ListAgents()
	var names []string
	for _, a := range agents {
		if strings.HasPrefix(a.Name, toComplete) {
			names = append(names, a.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// resolveParallel returns the number of parallel workers to use.
// Precedence: --parallel flag > config parallel > min(NumCPU, 8).
func resolveParallel() int {
	if parallelFlag > 0 {
		return parallelFlag
	}
	if configParallel > 0 {
		return configParallel
	}
	n := runtime.NumCPU()
	if n > 8 {
		n = 8
	}
	return n
}

// lockRepository acquires an exclusive repository lock for CLI mutating operations.
func lockRepository(repo *repository.Repository) error {
	return repo.LockWithOptions(repository.LockOptions{
		Exclusive:  true,
		BreakStale: breakLockFlag,
	})
}

// newAgentsCmd exposes agent management at top level (in addition to under `serve agent`).
func newAgentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Manage registered backup agents",
	}
	cmd.AddCommand(newServeAgentAddCmd())
	cmd.AddCommand(newServeAgentListCmd())
	cmd.AddCommand(newServeAgentRemoveCmd())
	cmd.AddCommand(newServeAgentReRegisterCmd())
	cmd.AddCommand(newServeAgentAssignRepoCmd())
	return cmd
}

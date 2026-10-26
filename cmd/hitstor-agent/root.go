package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/hooperits/hitstor/internal/bmr/bmrcli"
	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/logstore"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	serverFlag   string
	apiKeyFlag   string
	caCertFlag   string
	verboseCount int
	quietFlag    bool
	jsonFlag     bool
	configFlag   string

	// loadedConfig holds the parsed config file.
	loadedConfig config.Config
)

var rootCmd = &cobra.Command{
	Use:           "hitstor-agent",
	Short:         "Remote backup agent for HitStor",
	Long:          "HitStor Agent connects to a HitStor server and performs encrypted, deduplicated remote backups.",
	SilenceUsage:  true,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(os.Stdout, "hitstor-agent v%s (%s, %s)\n\n", Version, Commit, Date)
		cmd.Help()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Load YAML config.
		loadConfig()

		// Environment variable overrides (after config, before flag check).
		if env := os.Getenv("HITSTOR_AGENT_SERVER"); env != "" && serverFlag == "" {
			serverFlag = env
		}
		if env := os.Getenv("HITSTOR_AGENT_API_KEY"); env != "" && apiKeyFlag == "" {
			apiKeyFlag = env
		}
		if env := os.Getenv("HITSTOR_AGENT_CA_CERT"); env != "" && caCertFlag == "" {
			caCertFlag = env
		}

		// Setup structured logging.
		setupLogging()
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&serverFlag, "server", "", "server address (host:port)")
	rootCmd.PersistentFlags().StringVar(&apiKeyFlag, "api-key", "", "agent API key (64-char hex)")
	rootCmd.PersistentFlags().StringVar(&caCertFlag, "ca-cert", "", "path to CA certificate for TLS verification")
	rootCmd.PersistentFlags().CountVarP(&verboseCount, "verbose", "v", "increase log verbosity (repeat for more: -vv)")
	rootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "suppress non-error output")
	rootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "output in JSON format")
	rootCmd.PersistentFlags().StringVar(&configFlag, "config", "", "config file path (default: ~/.config/hitstor/config.yaml)")

	rootCmd.AddCommand(newPingCmd())
	rootCmd.AddCommand(newBackupCmd())
	rootCmd.AddCommand(newDaemonCmd())
	rootCmd.AddCommand(newShowIDCmd())
	rootCmd.AddCommand(newRecoverCmd())
	// Recovery media can only be built on the host it protects, and the agent
	// package is what protected hosts actually install.
	rootCmd.AddCommand(bmrcli.New(bmrcli.Options{
		JSON:       func() bool { return jsonFlag },
		ServerAddr: func() string { return serverFlag },
	}))
}

// setupLogging configures the global slog logger based on flags.
func setupLogging() {
	var level slog.Level
	switch {
	case quietFlag:
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

func initAgentLogStore(cfg config.Config) {
	if quietFlag {
		return
	}
	if err := logstore.InitFromConfig(config.LogStoreConfig(cfg)); err != nil {
		slog.Warn("logstore init failed", "error", err)
		return
	}
	level := slog.LevelWarn
	switch {
	case verboseCount >= 2:
		level = slog.LevelDebug
	case verboseCount == 1:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	var base slog.Handler
	if jsonFlag {
		base = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		base = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(logstore.NewTeeHandler(base, "agent")))
}

// loadConfig loads the YAML config file and applies agent-related values.
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
	if serverFlag == "" && cfg.AgentServer != "" {
		serverFlag = cfg.AgentServer
	}
	if apiKeyFlag == "" && cfg.AgentAPIKey != "" {
		apiKeyFlag = cfg.AgentAPIKey
	}
	if caCertFlag == "" && cfg.AgentCACert != "" {
		caCertFlag = cfg.AgentCACert
	}
}

// requireServer validates that the --server flag is set.
func requireServer() error {
	if serverFlag == "" {
		return fmt.Errorf("server address required: use --server <host:port> or set HITSTOR_AGENT_SERVER")
	}
	return nil
}

// requireAPIKey validates that the --api-key flag is set.
func requireAPIKey() error {
	if apiKeyFlag == "" {
		return fmt.Errorf("API key required: use --api-key <key> or set HITSTOR_AGENT_API_KEY")
	}
	return nil
}

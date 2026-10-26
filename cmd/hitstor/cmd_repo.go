package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hooperits/hitstor/internal/config"
	"github.com/hooperits/hitstor/internal/repository"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
)

func newRepoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Repository operations (multi-repo aware)",
	}
	cmd.AddCommand(newRepoListCmd())
	cmd.AddCommand(newRepoCreateCmd())
	cmd.AddCommand(newRepoDeleteCmd())
	cmd.AddCommand(newRepoHealthCmd())
	return cmd
}

func newRepoListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured repositories (from multi-repo config)",
		Args:  cobra.NoArgs,
		RunE:  runRepoList,
	}
}

func runRepoList(cmd *cobra.Command, args []string) error {
	cfg := loadedConfig
	if len(cfg.Repositories) == 0 {
		fmt.Fprintln(os.Stdout, "No repositories configured in multi-repo mode.")
		fmt.Fprintln(os.Stdout, "Use the dashboard Storage page, or ensure /etc/hitstor/config.yaml exists with a 'repositories:' list.")
		fmt.Fprintln(os.Stdout, "CLI on server: hitstor --config /etc/hitstor/config.yaml repo list")
		return nil
	}

	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg.Repositories)
	}

	color := ui.NewColorWriter(os.Stdout, quietFlag || jsonFlag)
	tbl := ui.NewTable(
		ui.Column{Header: "NAME", MinWidth: 12},
		ui.Column{Header: "TYPE", MinWidth: 8},
		ui.Column{Header: "LOCATION", MinWidth: 30},
		ui.Column{Header: "PASSWORD FILE"},
	)

	for _, rc := range cfg.Repositories {
		loc := rc.Path
		if rc.Type == "s3" {
			loc = fmt.Sprintf("%s/%s", rc.Endpoint, rc.Bucket)
		}
		pw := rc.PasswordFile
		if pw == "" {
			pw = "(not set)"
		}
		tbl.AddRow(color.Cyan(rc.Name), rc.Type, loc, pw)
	}

	tbl.Render(os.Stdout, color)
	fmt.Fprintln(os.Stdout, "\nUse --repo-name <name> with other commands, or manage via dashboard.")
	return nil
}

func newRepoHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Show repository operational health",
		Args:  cobra.NoArgs,
		RunE:  runRepoHealth,
	}
}

func runRepoHealth(cmd *cobra.Command, args []string) error {
	repo, be, err := openRepository()
	if err != nil {
		return err
	}
	defer be.Close()

	report, err := repo.Health()
	if err != nil {
		return fmt.Errorf("health: %w", err)
	}

	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(report)
	}

	color := ui.NewColorWriter(os.Stdout, quietFlag)
	if !quietFlag {
		fmt.Fprintf(os.Stdout, "Indexes:        %s\n", color.Cyan(fmt.Sprintf("%d", report.IndexCount)))
		if report.LatestIndexID != "" {
			fmt.Fprintf(os.Stdout, "Latest index:   %s\n", color.Cyan(report.LatestIndexID[:16]+"..."))
			fmt.Fprintf(os.Stdout, "Delta indexes:  %s\n", color.Cyan(fmt.Sprintf("%d", report.DeltaIndexCount)))
		}
		fmt.Fprintf(os.Stdout, "Snapshots:      %s\n", color.Cyan(fmt.Sprintf("%d", report.SnapshotCount)))
		fmt.Fprintf(os.Stdout, "Locks:          %s\n", color.Cyan(fmt.Sprintf("%d", report.LockCount)))
		fmt.Fprintf(os.Stdout, "Min free:       %s\n", color.Cyan(ui.FormatBytes(report.MinFreeReserve)))
		if report.FreeBytes > 0 || report.TotalBytes > 0 {
			fmt.Fprintf(os.Stdout, "Disk free:      %s / %s\n",
				color.Cyan(ui.FormatBytes(report.FreeBytes)),
				color.Cyan(ui.FormatBytes(report.TotalBytes)))
		}
		if report.SyncWrites {
			fmt.Fprintf(os.Stdout, "Sync writes:    %s\n", color.Green("enabled"))
		}
		for _, lock := range report.Locks {
			fmt.Fprintf(os.Stdout, "  lock: %s pid %d since %s\n",
				lock.Hostname, lock.PID, lock.Time.Format("2006-01-02 15:04:05"))
		}
	}
	return nil
}

// --- repo create (basic, local only for now) ---

func newRepoCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name> <path>",
		Short: "Create and initialize a new repository (local or s3, multi-repo)",
		Long:  "Create a new repository. For S3, provide --s3-endpoint, --s3-bucket and credentials via flags or --s3-* globals. The repo password is separate from S3 keys.",
		Args:  cobra.ExactArgs(2),
		RunE:  runRepoCreate,
	}
	cmd.Flags().String("password", "", "password for the new repo (or use --password)")
	cmd.Flags().String("type", "local", "local or s3")
	cmd.Flags().String("endpoint", "", "S3 endpoint (e.g. s3.example.com)")
	cmd.Flags().String("bucket", "", "S3 bucket")
	cmd.Flags().String("region", "us-east-1", "S3 region")
	cmd.Flags().String("access-key", "", "S3 access key (will be stored encrypted using repo key)")
	cmd.Flags().String("secret-key", "", "S3 secret key (will be stored encrypted)")
	return cmd
}

func runRepoCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	location := args[1]

	typ, _ := cmd.Flags().GetString("type")
	endpoint, _ := cmd.Flags().GetString("endpoint")
	bucket, _ := cmd.Flags().GetString("bucket")
	region, _ := cmd.Flags().GetString("region")
	accessKey, _ := cmd.Flags().GetString("access-key")
	secretKey, _ := cmd.Flags().GetString("secret-key")

	isS3 := typ == "s3" || endpoint != "" || bucket != ""
	if isS3 {
		if endpoint == "" || bucket == "" {
			return fmt.Errorf("for S3 repo, --endpoint and --bucket are required")
		}
	} else {
		if !filepath.IsAbs(location) {
			return fmt.Errorf("path must be absolute for local repo")
		}
	}

	// Check duplicate
	for _, rc := range loadedConfig.Repositories {
		if rc.Name == name {
			return fmt.Errorf("repository %q already exists", name)
		}
	}

	password := passwordFlag
	if password == "" {
		p, err := getPassword()
		if err != nil {
			return fmt.Errorf("password required for new repo: %w", err)
		}
		password = p
	}

	// Create password file next to config
	cfgPath := configPath()
	if cfgPath == "" {
		cfgPath = config.DefaultPath()
	}
	cfgDir := filepath.Dir(cfgPath)
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		return err
	}
	pwFile := filepath.Join(cfgDir, name+".password")
	if err := os.WriteFile(pwFile, []byte(password), 0600); err != nil {
		return fmt.Errorf("write password file: %w", err)
	}

	// Temporarily set for init
	oldRepo := repoFlag
	oldPwFile := passwordFileFlag
	oldS3AK := s3AccessKeyFlag
	oldS3SK := s3SecretKeyFlag
	oldS3Region := s3RegionFlag

	passwordFileFlag = pwFile
	if isS3 {
		s3AccessKeyFlag = accessKey
		s3SecretKeyFlag = secretKey
		s3RegionFlag = region
		// Use s3: form for openBackend
		ep := strings.TrimPrefix(endpoint, "https://")
		ep = strings.TrimPrefix(ep, "http://")
		repoFlag = fmt.Sprintf("s3:%s/%s", ep, bucket)
	} else {
		repoFlag = location
	}

	defer func() {
		repoFlag = oldRepo
		passwordFileFlag = oldPwFile
		s3AccessKeyFlag = oldS3AK
		s3SecretKeyFlag = oldS3SK
		s3RegionFlag = oldS3Region
	}()

	progress := ui.NewProgress(os.Stdout, jsonFlag, quietFlag)
	be, err := openBackend()
	if err != nil {
		os.Remove(pwFile)
		return fmt.Errorf("backend: %w", err)
	}
	defer be.Close()

	repo, err := repository.Init(be, password)
	if err != nil {
		os.Remove(pwFile)
		return fmt.Errorf("init: %w", err)
	}

	// Add to loaded config (in memory) and save
	rc := config.RepoConfig{
		Name:         name,
		PasswordFile: pwFile,
	}
	if isS3 {
		rc.Type = "s3"
		rc.Endpoint = endpoint
		rc.Bucket = bucket
		rc.Region = region
		if accessKey != "" {
			enc, err := config.EncryptCredential(repo.Key().EncryptKey, accessKey)
			if err != nil {
				return fmt.Errorf("encrypt s3 access key: %w", err)
			}
			rc.AccessKeyEncrypted = enc
		}
		if secretKey != "" {
			enc, err := config.EncryptCredential(repo.Key().EncryptKey, secretKey)
			if err != nil {
				return fmt.Errorf("encrypt s3 secret key: %w", err)
			}
			rc.SecretKeyEncrypted = enc
		}
	} else {
		rc.Type = "local"
		rc.Path = location
	}
	loadedConfig.Repositories = append(loadedConfig.Repositories, rc)

	if err := config.Save(loadedConfig); err != nil {
		return fmt.Errorf("save config with new repo: %w", err)
	}

	loc := location
	if isS3 {
		loc = fmt.Sprintf("%s/%s", endpoint, bucket)
	}
	progress.InitComplete(repo.Cfg().ID, loc)
	fmt.Fprintf(os.Stdout, "Added repository %q to config. Use with --repo-name %s\n", name, name)
	return nil
}

func newRepoDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Remove a repository from the multi-repo configuration (does not delete data)",
		Args:  cobra.ExactArgs(1),
		RunE:  runRepoDelete,
	}
}

func runRepoDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	found := false
	newRepos := make([]config.RepoConfig, 0, len(loadedConfig.Repositories))
	for _, rc := range loadedConfig.Repositories {
		if rc.Name == name {
			found = true
			// Optionally remove the password file
			if rc.PasswordFile != "" {
				_ = os.Remove(rc.PasswordFile)
			}
			continue
		}
		newRepos = append(newRepos, rc)
	}

	if !found {
		return fmt.Errorf("repository %q not found", name)
	}

	loadedConfig.Repositories = newRepos
	if err := config.Save(loadedConfig); err != nil {
		return fmt.Errorf("save config after delete: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Removed repository %q from config (data on disk was not deleted).\n", name)
	return nil
}
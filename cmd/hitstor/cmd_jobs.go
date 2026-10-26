package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/hooperits/hitstor/internal/server"
	"github.com/hooperits/hitstor/internal/ui"
	"github.com/spf13/cobra"
)

func newJobsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "Manage scheduled backup jobs (multi-repo aware)",
		Long:  "Manage scheduled backup jobs. list/create + new: orphans, reattach. Full CRUD in dashboard.",
	}
	cmd.AddCommand(newJobsListCmd())
	cmd.AddCommand(newJobsCreateCmd())
	cmd.AddCommand(newJobsOrphansCmd())
	cmd.AddCommand(newJobsReattachCmd())
	// TODO: delete, update, run-now, cancel
	return cmd
}

func newJobsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured backup jobs",
		Args:  cobra.NoArgs,
		RunE:  runJobsList,
	}
	cmd.Flags().String("repo-name", "", "target repository name (to load its job registry)")
	cmd.Flags().String("agent", "", "filter jobs for this agent name")
	cmd.Flags().Bool("orphans", false, "list only orphaned jobs (AgentName has no registered agent)")
	return cmd
}

func runJobsList(cmd *cobra.Command, args []string) error {
	repoName, _ := cmd.Flags().GetString("repo-name")
	agentFilter, _ := cmd.Flags().GetString("agent")
	orphansOnly, _ := cmd.Flags().GetBool("orphans")

	color := ui.NewColorWriter(os.Stdout, quietFlag || jsonFlag)

	if repoName != "" {
		// Try to load real jobs from the named repo's registry
		old := repoNameFlag
		repoNameFlag = repoName
		defer func() { repoNameFlag = old }()

		repo, be, err := openRepository()
		if err != nil {
			return fmt.Errorf("open repo %s for jobs: %w", repoName, err)
		}
		defer be.Close()

		basePath, err := backendBasePath(be)
		if err != nil {
			// non-local, just note
			fmt.Fprintf(os.Stdout, "Registry listing for non-local repo %s not supported yet.\n", repoName)
			return nil
		}

		registry, err := server.LoadAgentRegistry(repo.Key().EncryptKey, basePath)
		if err != nil {
			return fmt.Errorf("load registry: %w", err)
		}

		var jobs []server.BackupJob
		if orphansOnly {
			jobs = registry.ListOrphanJobs()
		} else if agentFilter != "" {
			jobs = registry.ListJobsByAgent(agentFilter)
		} else {
			jobs = registry.ListJobs()
		}

		if jsonFlag {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(jobs)
		}

		if len(jobs) == 0 {
			if orphansOnly {
				fmt.Fprintf(os.Stdout, "No orphaned jobs for repo %q.\n", repoName)
			} else if agentFilter != "" {
				fmt.Fprintf(os.Stdout, "No jobs for agent %q in repo %q.\n", agentFilter, repoName)
			} else {
				fmt.Fprintf(os.Stdout, "No jobs registered for repo %q.\n", repoName)
			}
			return nil
		}

		tbl := ui.NewTable(
			ui.Column{Header: "ID", MinWidth: 10},
			ui.Column{Header: "NAME", MinWidth: 15},
			ui.Column{Header: "TYPE", MinWidth: 10},
			ui.Column{Header: "AGENT", MinWidth: 15},
			ui.Column{Header: "SCHEDULE", MinWidth: 20},
		)
		for _, j := range jobs {
			sched := ""
			if j.CronExpression != "" {
				sched = j.CronExpression
			} else if j.ScheduleHours > 0 {
				sched = fmt.Sprintf("every %dh", j.ScheduleHours)
			}
			tbl.AddRow(j.ID, color.Cyan(j.Name), j.JobType, j.AgentName, sched)
		}
		tbl.Render(os.Stdout, color)
		return nil
	}

	// Fallback / no repo-name provided
	fmt.Fprintln(os.Stdout, "Backup jobs management (CLI)")
	fmt.Fprintln(os.Stdout, "Note: Use --repo-name <name> to list jobs from a specific repo's registry.")
	fmt.Fprintln(os.Stdout, "Full management is available in the dashboard.")
	fmt.Fprintln(os.Stdout, "New: --agent, --orphans, plus 'jobs orphans' and 'jobs reattach' subcommands.")

	if jsonFlag {
		fmt.Fprintln(os.Stdout, "[]")
		return nil
	}

	tbl := ui.NewTable(
		ui.Column{Header: "NAME", MinWidth: 15},
		ui.Column{Header: "TYPE", MinWidth: 10},
		ui.Column{Header: "REPO", MinWidth: 15},
		ui.Column{Header: "SCHEDULE", MinWidth: 20},
	)
	tbl.AddRow(color.Cyan("(use --repo-name or dashboard)"), "files", "(any)", "interval or cron")
	tbl.Render(os.Stdout, color)

	fmt.Fprintln(os.Stdout, "\nSee 'hitstor serve' for registration, and dashboard for full job management.")
	return nil
}

func newJobsCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Validate and print a new backup job definition (full persistence is dashboard-first)",
		RunE:  runJobsCreate,
	}
	cmd.Flags().String("name", "", "job name (required)")
	cmd.Flags().String("repo-name", "", "target repository name (required)")
	cmd.Flags().String("agent-name", "", "agent name (for agent jobs)")
	cmd.Flags().String("job-type", "files", "job type (only \"files\" is supported)")
	cmd.Flags().StringArray("path", nil, "paths to back up (repeatable)")
	cmd.Flags().Int("interval-hours", 0, "schedule interval in hours")
	cmd.Flags().String("cron", "", "cron expression")
	cmd.Flags().String("timezone", "Local", "IANA timezone")
	// Add more flags as needed (db_type, etc.)
	return cmd
}

func runJobsCreate(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	repoName, _ := cmd.Flags().GetString("repo-name")
	agentName, _ := cmd.Flags().GetString("agent-name")
	jobType, _ := cmd.Flags().GetString("job-type")
	paths, _ := cmd.Flags().GetStringArray("path")
	interval, _ := cmd.Flags().GetInt("interval-hours")
	cron, _ := cmd.Flags().GetString("cron")
	timezone, _ := cmd.Flags().GetString("timezone")

	if name == "" || repoName == "" {
		return fmt.Errorf("--name and --repo-name are required")
	}

	// Build proper job struct
	var idBytes [8]byte
	rand.Read(idBytes[:])

	job := server.BackupJob{
		ID:              hex.EncodeToString(idBytes[:]),
		Name:            name,
		AgentName:       agentName,
		RepoName:        repoName,
		Paths:           paths,
		ScheduleEnabled: cron != "" || interval > 0,
		ScheduleHours:   interval,
		CronExpression:  cron,
		Timezone:        timezone,
		JobType:         jobType,
		CreatedAt:       time.Now(),
	}

	// Temporarily select the target repo for open/ registry load
	oldRepoName := repoNameFlag
	repoNameFlag = repoName
	defer func() { repoNameFlag = oldRepoName }()

	repo, be, err := openRepository()
	if err != nil {
		return fmt.Errorf("open target repo %q for registry: %w", repoName, err)
	}
	defer be.Close()

	basePath, err := backendBasePath(be)
	if err != nil {
		return fmt.Errorf("jobs registry currently requires a local backend: %w", err)
	}

	registry, err := server.LoadAgentRegistry(repo.Key().EncryptKey, basePath)
	if err != nil {
		return fmt.Errorf("load agent registry for %s: %w", repoName, err)
	}

	if err := registry.AddJob(job); err != nil {
		return fmt.Errorf("add job: %w", err)
	}

	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(job)
	}

	fmt.Printf("Job created (id=%s)\n", job.ID)
	fmt.Printf("  Name: %s\n", name)
	fmt.Printf("  Repo: %s\n", repoName)
	if agentName != "" {
		fmt.Printf("  Agent: %s\n", agentName)
	}
	fmt.Printf("  Type: %s\n", jobType)
	if len(paths) > 0 {
		fmt.Printf("  Paths: %v\n", paths)
	}
	if interval > 0 {
		fmt.Printf("  Schedule: every %d hours\n", interval)
	}
	if cron != "" {
		fmt.Printf("  Cron: %s (%s)\n", cron, timezone)
	}

	return nil
}

// TODO future:
// - delete, update, run-now, cancel
// - Real list by loading registry for the repo (requires password + key)
// Leverage internal/server for loading jobs when possible.

// newJobsOrphansCmd lists orphaned jobs (whose AgentName has no matching registered agent).
func newJobsOrphansCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orphans",
		Short: "List orphaned backup jobs (agent no longer registered)",
		Args:  cobra.NoArgs,
		RunE:  runJobsOrphans,
	}
	cmd.Flags().String("repo-name", "", "target repository name (required)")
	return cmd
}

func runJobsOrphans(cmd *cobra.Command, args []string) error {
	repoName, _ := cmd.Flags().GetString("repo-name")
	if repoName == "" {
		return fmt.Errorf("--repo-name is required for orphans")
	}

	old := repoNameFlag
	repoNameFlag = repoName
	defer func() { repoNameFlag = old }()

	repo, be, err := openRepository()
	if err != nil {
		return fmt.Errorf("open repo %s: %w", repoName, err)
	}
	defer be.Close()

	basePath, err := backendBasePath(be)
	if err != nil {
		return fmt.Errorf("orphans requires local backend: %w", err)
	}

	registry, err := server.LoadAgentRegistry(repo.Key().EncryptKey, basePath)
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}

	jobs := registry.ListOrphanJobs()
	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(jobs)
	}

	if len(jobs) == 0 {
		fmt.Fprintf(os.Stdout, "No orphaned jobs in repo %q.\n", repoName)
		return nil
	}

	color := ui.NewColorWriter(os.Stdout, quietFlag || jsonFlag)
	tbl := ui.NewTable(
		ui.Column{Header: "ID", MinWidth: 10},
		ui.Column{Header: "NAME", MinWidth: 15},
		ui.Column{Header: "AGENT (ORPHANED)", MinWidth: 20},
		ui.Column{Header: "REPO", MinWidth: 15},
	)
	for _, j := range jobs {
		tbl.AddRow(j.ID, j.Name, j.AgentName, repoName)
	}
	tbl.Render(os.Stdout, color)
	return nil
}

// newJobsReattachCmd re-links jobs to a (re-registered) agent.
func newJobsReattachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reattach",
		Short: "Re-attach orphaned or specific jobs to an agent name",
		Args:  cobra.NoArgs,
		RunE:  runJobsReattach,
	}
	cmd.Flags().String("repo-name", "", "target repository name (required)")
	cmd.Flags().String("agent", "", "target agent name (required)")
	cmd.Flags().StringArray("job-id", nil, "specific job ID(s) to reattach (repeatable)")
	cmd.Flags().Bool("all-orphans", false, "reattach all currently orphaned jobs for this agent")
	return cmd
}

func runJobsReattach(cmd *cobra.Command, args []string) error {
	repoName, _ := cmd.Flags().GetString("repo-name")
	targetAgent, _ := cmd.Flags().GetString("agent")
	jobIDs, _ := cmd.Flags().GetStringArray("job-id")
	allOrphans, _ := cmd.Flags().GetBool("all-orphans")

	if repoName == "" || targetAgent == "" {
		return fmt.Errorf("--repo-name and --agent are required")
	}
	if len(jobIDs) == 0 && !allOrphans {
		return fmt.Errorf("provide --job-id or --all-orphans")
	}

	old := repoNameFlag
	repoNameFlag = repoName
	defer func() { repoNameFlag = old }()

	repo, be, err := openRepository()
	if err != nil {
		return fmt.Errorf("open repo: %w", err)
	}
	defer be.Close()

	basePath, err := backendBasePath(be)
	if err != nil {
		return fmt.Errorf("reattach requires local backend: %w", err)
	}

	registry, err := server.LoadAgentRegistry(repo.Key().EncryptKey, basePath)
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}

	idsToUse := jobIDs
	if allOrphans {
		orphans := registry.ListOrphanJobs()
		for _, o := range orphans {
			if o.AgentName != "" { // any orphan, or could filter later
				idsToUse = append(idsToUse, o.ID)
			}
		}
	}

	if err := registry.ReattachJobs(targetAgent, idsToUse); err != nil {
		return fmt.Errorf("reattach: %w", err)
	}

	fmt.Printf("Reattached %d job(s) to agent %q in repo %q\n", len(idsToUse), targetAgent, repoName)
	return nil
}

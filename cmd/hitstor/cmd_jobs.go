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
		Long: "Manage scheduled backup jobs: list, create, orphans, reattach. Full management is in the dashboard.\n\n" +
			"Jobs live in the server's job registry, kept in the first configured repository by name (the one\n" +
			"`hitstor serve` uses), whatever repository each job backs up to.",
	}
	cmd.AddCommand(newJobsListCmd())
	cmd.AddCommand(newJobsCreateCmd())
	cmd.AddCommand(newJobsOrphansCmd())
	cmd.AddCommand(newJobsReattachCmd())
	return cmd
}

func newJobsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured backup jobs",
		Args:  cobra.NoArgs,
		RunE:  runJobsList,
	}
	cmd.Flags().String("repo-name", "", "only jobs that back up to this repository")
	cmd.Flags().String("agent", "", "filter jobs for this agent name")
	cmd.Flags().Bool("orphans", false, "list only orphaned jobs (AgentName has no registered agent)")
	return cmd
}

func runJobsList(cmd *cobra.Command, args []string) error {
	repoName, _ := cmd.Flags().GetString("repo-name")
	agentFilter, _ := cmd.Flags().GetString("agent")
	orphansOnly, _ := cmd.Flags().GetBool("orphans")

	color := ui.NewColorWriter(os.Stdout, quietFlag || jsonFlag)

	registry, closeRegistry, err := openJobRegistry()
	if err != nil {
		return err
	}
	defer closeRegistry()

	var jobs []server.BackupJob
	if orphansOnly {
		jobs = registry.ListOrphanJobs()
	} else if agentFilter != "" {
		jobs = registry.ListJobsByAgent(agentFilter)
	} else {
		jobs = registry.ListJobs()
	}
	jobs = filterJobsByRepo(jobs, repoName)

	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(jobs)
	}

	if len(jobs) == 0 {
		fmt.Fprintln(os.Stdout, "No matching jobs.")
		return nil
	}

	tbl := ui.NewTable(
		ui.Column{Header: "ID", MinWidth: 10},
		ui.Column{Header: "NAME", MinWidth: 15},
		ui.Column{Header: "TYPE", MinWidth: 10},
		ui.Column{Header: "AGENT", MinWidth: 15},
		ui.Column{Header: "REPO", MinWidth: 10},
		ui.Column{Header: "SCHEDULE", MinWidth: 20},
	)
	for _, j := range jobs {
		sched := ""
		if j.CronExpression != "" {
			sched = j.CronExpression
		} else if j.ScheduleHours > 0 {
			sched = fmt.Sprintf("every %dh", j.ScheduleHours)
		}
		tbl.AddRow(j.ID, color.Cyan(j.Name), j.JobType, j.AgentName, j.RepoName, sched)
	}
	tbl.Render(os.Stdout, color)
	return nil
}

func newJobsCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a backup job",
		Args:  cobra.NoArgs,
		RunE:  runJobsCreate,
	}
	cmd.Flags().String("name", "", "job name (required)")
	cmd.Flags().String("repo-name", "", "repository the job backs up to (required)")
	cmd.Flags().String("agent-name", "", "agent name (for agent jobs)")
	cmd.Flags().String("job-type", "files", "job type (only \"files\" is supported)")
	cmd.Flags().StringArray("path", nil, "paths to back up (repeatable)")
	cmd.Flags().Int("interval-hours", 0, "schedule interval in hours")
	cmd.Flags().String("cron", "", "cron expression")
	cmd.Flags().String("timezone", "Local", "IANA timezone")
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
	if err := checkRepoName(repoName); err != nil {
		return err
	}

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

	registry, closeRegistry, err := openJobRegistry()
	if err != nil {
		return err
	}
	defer closeRegistry()

	if err := registry.AddJob(job); err != nil {
		return fmt.Errorf("add job: %w", err)
	}

	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(job)
	}
	fmt.Fprintf(os.Stdout, "Job %q created (ID %s), backing up to repository %q.\n", name, job.ID, repoName)
	return nil
}

func newJobsOrphansCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orphans",
		Short: "List jobs whose agent is no longer registered",
		Args:  cobra.NoArgs,
		RunE:  runJobsOrphans,
	}
	cmd.Flags().String("repo-name", "", "only jobs that back up to this repository")
	return cmd
}

func runJobsOrphans(cmd *cobra.Command, args []string) error {
	repoName, _ := cmd.Flags().GetString("repo-name")

	registry, closeRegistry, err := openJobRegistry()
	if err != nil {
		return err
	}
	defer closeRegistry()

	jobs := filterJobsByRepo(registry.ListOrphanJobs(), repoName)
	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(jobs)
	}

	if len(jobs) == 0 {
		fmt.Fprintln(os.Stdout, "No orphaned jobs.")
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
		tbl.AddRow(j.ID, j.Name, j.AgentName, j.RepoName)
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
	cmd.Flags().String("repo-name", "", "with --all-orphans, only jobs that back up to this repository")
	cmd.Flags().String("agent", "", "target agent name (required)")
	cmd.Flags().StringArray("job-id", nil, "specific job ID(s) to reattach (repeatable)")
	cmd.Flags().Bool("all-orphans", false, "reattach all currently orphaned jobs to this agent")
	return cmd
}

func runJobsReattach(cmd *cobra.Command, args []string) error {
	repoName, _ := cmd.Flags().GetString("repo-name")
	targetAgent, _ := cmd.Flags().GetString("agent")
	jobIDs, _ := cmd.Flags().GetStringArray("job-id")
	allOrphans, _ := cmd.Flags().GetBool("all-orphans")

	if targetAgent == "" {
		return fmt.Errorf("--agent is required")
	}
	if len(jobIDs) == 0 && !allOrphans {
		return fmt.Errorf("provide --job-id or --all-orphans")
	}

	registry, closeRegistry, err := openJobRegistry()
	if err != nil {
		return err
	}
	defer closeRegistry()

	idsToUse := jobIDs
	if allOrphans {
		for _, o := range filterJobsByRepo(registry.ListOrphanJobs(), repoName) {
			idsToUse = append(idsToUse, o.ID)
		}
	}

	if err := registry.ReattachJobs(targetAgent, idsToUse); err != nil {
		return fmt.Errorf("reattach: %w", err)
	}

	fmt.Printf("Reattached %d job(s) to agent %q\n", len(idsToUse), targetAgent)
	return nil
}

// openJobRegistry loads the registry `hitstor serve` reads jobs from, which
// lives in the server's repository (see controlRepoName), whatever
// repository the jobs back up to.
func openJobRegistry() (*server.AgentRegistry, func(), error) {
	old := repoNameFlag
	repoNameFlag = ""
	repo, be, err := openControlRepository()
	repoNameFlag = old
	if err != nil {
		return nil, nil, err
	}
	basePath, err := backendBasePath(be)
	if err != nil {
		be.Close()
		return nil, nil, fmt.Errorf("the job registry requires a local repository: %w", err)
	}
	registry, err := server.LoadAgentRegistry(repo.Key().EncryptKey, basePath)
	if err != nil {
		be.Close()
		return nil, nil, fmt.Errorf("load job registry: %w", err)
	}
	return registry, func() { be.Close() }, nil
}

// checkRepoName fails when name is not a configured repository.
func checkRepoName(name string) error {
	if len(loadedConfig.Repositories) == 0 {
		return nil
	}
	for _, rc := range loadedConfig.Repositories {
		if rc.Name == name {
			return nil
		}
	}
	return fmt.Errorf("repository %q not found in config", name)
}

// filterJobsByRepo keeps the jobs that back up to repo (all when empty).
func filterJobsByRepo(jobs []server.BackupJob, repo string) []server.BackupJob {
	if repo == "" {
		return jobs
	}
	out := jobs[:0:0]
	for _, j := range jobs {
		if j.RepoName == repo {
			out = append(out, j)
		}
	}
	return out
}

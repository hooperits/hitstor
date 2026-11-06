package agent

// AgentJobRunnable reports whether the agent can execute a scheduled job.
// Filesystem jobs (job type "" or "files") require at least one path;
// workload jobs require a plugin and at least one selected object. Whether
// the plugin is installed is checked when the job runs, so the failure is
// logged instead of the job being silently skipped.
func AgentJobRunnable(job JobSchedule) bool {
	switch job.JobType {
	case "", "files":
		return len(job.Paths) > 0
	case "workload":
		return job.Plugin != "" && len(job.Selection) > 0
	default:
		return false
	}
}

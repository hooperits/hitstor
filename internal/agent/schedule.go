package agent

// AgentJobRunnable reports whether the agent can execute a scheduled job.
// HitStor agents only run filesystem backups (job type "" or "files");
// filesystem jobs require at least one path.
func AgentJobRunnable(job JobSchedule) bool {
	switch job.JobType {
	case "", "files":
		return len(job.Paths) > 0
	default:
		return false
	}
}

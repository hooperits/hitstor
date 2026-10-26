package agent

import "testing"

func TestAgentJobRunnable(t *testing.T) {
	tests := []struct {
		name string
		job  JobSchedule
		want bool
	}{
		{
			name: "files with paths",
			job:  JobSchedule{JobType: "files", Paths: []string{"/data"}},
			want: true,
		},
		{
			name: "empty job type with paths",
			job:  JobSchedule{Paths: []string{"/data"}},
			want: true,
		},
		{
			name: "files without paths",
			job:  JobSchedule{JobType: "files"},
			want: false,
		},
		{
			name: "database jobs are not runnable (filesystem product focus)",
			job:  JobSchedule{JobType: "database"},
			want: false,
		},
		{
			name: "cloudvm skipped by agent",
			job:  JobSchedule{JobType: "cloudvm", Paths: []string{"vm-1"}},
			want: false,
		},
		{
			name: "m365 skipped by agent",
			job:  JobSchedule{JobType: "m365"},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AgentJobRunnable(tc.job); got != tc.want {
				t.Errorf("AgentJobRunnable() = %v, want %v", got, tc.want)
			}
		})
	}
}

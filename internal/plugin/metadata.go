package plugin

// ContributionType classifies dashboard features a plugin registers.
type ContributionType string

const (
	ContributionJobType         ContributionType = "job_type"
	ContributionIntegration     ContributionType = "integration"
	ContributionRestoreType     ContributionType = "restore_type"
	ContributionSettingsSection ContributionType = "settings_section"
)

// Contribution describes a dashboard feature registered by an installed plugin.
type Contribution struct {
	Type    ContributionType `json:"type"`
	ID      string           `json:"id"`
	Label   string           `json:"label"`
	Actions []string         `json:"actions"`
}

// PluginMetadata is returned by collector plugins via GetMetadata.
type PluginMetadata struct {
	Name            string         `json:"name"`
	Version         string         `json:"version"`
	MinCoreVersion  string         `json:"min_core_version"`
	ProtocolVersion int32          `json:"protocol_version"`
	Contributions   []Contribution `json:"contributions"`
}

// Plugin status values persisted in installed-plugins.json.
const (
	PluginStatusActive       = "active"
	PluginStatusDegraded     = "degraded"
	PluginStatusUnregistered = "unregistered"
)

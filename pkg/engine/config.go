package engine
package engine

// DrillConfig represents the configuration for a single drill.
type DrillConfig struct {
	Name     string       `yaml:"name"`
	Provider string       `yaml:"provider"`
	Schedule string       `yaml:"schedule"`
	Backup   BackupConfig `yaml:"backup"`
	Restore  RestoreSpec  `yaml:"restore"`
	Validate []Check      `yaml:"validate"`
	Alerts   []AlertSpec  `yaml:"alerts"`
}

// BackupConfig defines where and how to find the backup.
type BackupConfig struct {
	Tool   string     `yaml:"tool"`
	Stanza string     `yaml:"stanza"`
	Source string     `yaml:"source"`
	Repo   RepoConfig `yaml:"repo"`
}

// RepoConfig defines backup storage location.
type RepoConfig struct {
	Type     string `yaml:"type"`
	Bucket   string `yaml:"bucket"`
	Endpoint string `yaml:"endpoint"`
	Prefix   string `yaml:"prefix"`
	Region   string `yaml:"region"`
}

// RestoreSpec defines how to restore.
type RestoreSpec struct {
	Target    string        `yaml:"target"`
	Container ContainerConf `yaml:"container"`
	Timeout   string        `yaml:"timeout"`
}

// ContainerConf defines the ephemeral container spec.
type ContainerConf struct {
	Image     string       `yaml:"image"`
	Resources ResourceConf `yaml:"resources"`
}

// ResourceConf defines container resource limits.
type ResourceConf struct {
	Memory string `yaml:"memory"`
	CPU    string `yaml:"cpu"`
}

// Check defines a validation check.
type Check struct {
	Type   string `yaml:"type"`
	Name   string `yaml:"name"`
	SQL    string `yaml:"sql,omitempty"`
	Keys   []string `yaml:"keys,omitempty"`
	Expect string `yaml:"expect"`
}

// AlertSpec defines where to send alerts.
type AlertSpec struct {
	Type     string `yaml:"type"`
	Endpoint string `yaml:"endpoint,omitempty"`
	URL      string `yaml:"url,omitempty"`
}

// Config is the top-level configuration.
type Config struct {
	Drills    []DrillConfig `yaml:"drills"`
	Metrics   MetricsConfig `yaml:"metrics"`
	Reporting ReportConfig  `yaml:"reporting"`
}

// MetricsConfig defines Prometheus metrics settings.
type MetricsConfig struct {
	Prometheus struct {
		Enabled     bool              `yaml:"enabled"`
		Pushgateway string            `yaml:"pushgateway"`
		Labels      map[string]string `yaml:"labels"`
	} `yaml:"prometheus"`
}

// ReportConfig defines reporting settings.
type ReportConfig struct {
	Format    []string `yaml:"format"`
	Output    string   `yaml:"output"`
	Retention string   `yaml:"retention"`
}

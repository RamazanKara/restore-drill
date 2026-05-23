package engine

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// envVarPattern matches ${VAR_NAME} or ${VAR_NAME:-default} patterns.
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

var supportedBackupTools = map[string]map[string]struct{}{
	"postgres": {
		"pg_dump":    {},
		"pg_restore": {},
		"pgbackrest": {},
		"wal-g":      {},
		"walg":       {},
	},
	"mysql": {
		"mysqldump":   {},
		"xtrabackup":  {},
		"mariabackup": {},
	},
	"redis": {
		"rdb": {},
		"aof": {},
	},
}

var supportedRepoTypes = map[string]struct{}{
	"s3":            {},
	"s3-compatible": {},
}

var supportedCheckTypes = map[string]struct{}{
	"query":      {},
	"sql":        {},
	"schema":     {},
	"freshness":  {},
	"key_count":  {},
	"key_sample": {},
	"row_count":  {},
	"extensions": {},
}

var sqlBackedCheckTypes = map[string]struct{}{
	"query":     {},
	"sql":       {},
	"schema":    {},
	"freshness": {},
	"row_count": {},
}

// LoadConfig reads and parses a drill config from the given file path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- config path is an explicit user CLI argument.
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	return ParseConfig(data)
}

// ParseConfig parses raw YAML bytes into a Config, performing environment
// variable interpolation and validation.
func ParseConfig(data []byte) (*Config, error) {
	expanded := interpolateEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// interpolateEnv replaces ${VAR} and ${VAR:-default} with environment values.
func interpolateEnv(input string) string {
	return envVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := envVarPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		varName := parts[1]
		defaultVal := ""
		if len(parts) >= 3 {
			defaultVal = parts[2]
		}

		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		return defaultVal
	})
}

// validateConfig checks that the config is semantically valid.
func validateConfig(cfg *Config) error {
	if len(cfg.Drills) == 0 {
		return fmt.Errorf("config: at least one drill must be defined")
	}

	seen := make(map[string]struct{})
	for i, drill := range cfg.Drills {
		if drill.Name == "" {
			return fmt.Errorf("config: drill[%d] must have a name", i)
		}
		if _, exists := seen[drill.Name]; exists {
			return fmt.Errorf("config: duplicate drill name %q", drill.Name)
		}
		seen[drill.Name] = struct{}{}

		if drill.Provider == "" {
			return fmt.Errorf("config: drill %q must specify a provider", drill.Name)
		}

		tools, ok := supportedBackupTools[drill.Provider]
		if !ok {
			return fmt.Errorf("config: drill %q has unknown provider %q", drill.Name, drill.Provider)
		}

		if drill.Backup.Tool == "" {
			return fmt.Errorf("config: drill %q must specify backup.tool", drill.Name)
		}
		if _, ok := tools[drill.Backup.Tool]; !ok {
			return fmt.Errorf("config: drill %q has unsupported backup.tool %q for provider %q", drill.Name, drill.Backup.Tool, drill.Provider)
		}
		if drill.Backup.Source == "" && drill.Backup.Repo.Type == "" {
			return fmt.Errorf("config: drill %q must specify backup.source or backup.repo.type", drill.Name)
		}
		if drill.Backup.Repo.Type != "" {
			if _, ok := supportedRepoTypes[drill.Backup.Repo.Type]; !ok {
				return fmt.Errorf("config: drill %q has unsupported backup.repo.type %q", drill.Name, drill.Backup.Repo.Type)
			}
			if drill.Backup.Repo.Bucket == "" {
				return fmt.Errorf("config: drill %q must specify backup.repo.bucket when backup.repo.type is set", drill.Name)
			}
			if drill.Backup.Repo.Prefix == "" {
				return fmt.Errorf("config: drill %q must specify backup.repo.prefix when backup.repo.type is set", drill.Name)
			}
		}

		if drill.Restore.Container.Image == "" {
			return fmt.Errorf("config: drill %q must specify restore.container.image", drill.Name)
		}

		if drill.Restore.Timeout != "" {
			if _, err := time.ParseDuration(drill.Restore.Timeout); err != nil {
				return fmt.Errorf("config: drill %q has invalid timeout %q: %w", drill.Name, drill.Restore.Timeout, err)
			}
		}

		if err := validateChecks(drill.Name, drill.Validate); err != nil {
			return err
		}
	}

	return nil
}

// validateChecks validates the check definitions for a drill.
func validateChecks(drillName string, checks []Check) error {
	for i, check := range checks {
		if check.Name == "" {
			return fmt.Errorf("config: drill %q check[%d] must have a name", drillName, i)
		}

		if _, ok := supportedCheckTypes[check.Type]; !ok {
			return fmt.Errorf("config: drill %q check %q has unknown type %q", drillName, check.Name, check.Type)
		}

		if _, ok := sqlBackedCheckTypes[check.Type]; ok && check.SQL == "" {
			return fmt.Errorf("config: drill %q check %q of type %q must have sql", drillName, check.Name, check.Type)
		}

		if check.Type == "key_sample" && len(check.Keys) == 0 {
			return fmt.Errorf("config: drill %q check %q of type 'key_sample' must have keys", drillName, check.Name)
		}

		if check.Expect == "" {
			return fmt.Errorf("config: drill %q check %q must have an expect expression", drillName, check.Name)
		}
	}
	return nil
}

// DrillTimeout returns the parsed timeout or a default of 10 minutes.
func (d *DrillConfig) DrillTimeout() time.Duration {
	if d.Restore.Timeout == "" {
		return 10 * time.Minute
	}
	dur, err := time.ParseDuration(d.Restore.Timeout)
	if err != nil {
		return 10 * time.Minute
	}
	return dur
}

// GetPorts returns the default ports to expose for the provider.
func GetDefaultPorts(provider string) []int {
	switch strings.ToLower(provider) {
	case "postgres":
		return []int{5432}
	case "mysql":
		return []int{3306}
	case "redis":
		return []int{6379}
	case "etcd":
		return []int{2379}
	default:
		return nil
	}
}

package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// envVarPattern matches ${VAR_NAME} or ${VAR_NAME:-default} patterns.
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

var yamlLineBreaks = regexp.MustCompile("\r\n|[\r\n\u0085\u2028\u2029]")

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
	"etcd": {
		"snapshot": {},
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
	"key_get":    {},
	"row_count":  {},
	"extensions": {},
}

var supportedCheckTypesByProvider = map[string]map[string]struct{}{
	"postgres": {
		"query":      {},
		"sql":        {},
		"schema":     {},
		"freshness":  {},
		"row_count":  {},
		"extensions": {},
	},
	"mysql": {
		"query":     {},
		"sql":       {},
		"schema":    {},
		"freshness": {},
		"row_count": {},
	},
	"redis": {
		"query":      {},
		"key_count":  {},
		"key_sample": {},
	},
	"etcd": {
		"query":     {},
		"key_count": {},
		"key_get":   {},
	},
}

// keyBackedCheckTypes require a single key or prefix in the check's key field.
var keyBackedCheckTypes = map[string]struct{}{
	"key_get": {},
}

var sqlBackedCheckTypes = map[string]struct{}{
	"query":     {},
	"sql":       {},
	"schema":    {},
	"freshness": {},
	"row_count": {},
}

var supportedAlertTypes = map[string]struct{}{
	"webhook":    {},
	"slack":      {},
	"prometheus": {},
}

// supportedAlertConditions gate when an alert fires. Empty means "always".
var supportedAlertConditions = map[string]struct{}{
	"":        {},
	"always":  {},
	"failure": {},
}

var supportedReportFormats = map[string]struct{}{
	"table":    {},
	"json":     {},
	"html":     {},
	"markdown": {},
}

const defaultReportRetention = 90 * 24 * time.Hour

// LoadConfig reads and parses a drill config from the given file path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- config path is an explicit user CLI argument.
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg, err := ParseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// ParseConfig parses raw YAML bytes into a Config, performing environment
// variable interpolation and validation.
func ParseConfig(data []byte) (*Config, error) {
	expanded, sourceLines := interpolateEnv(string(data))
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(expanded), &document); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	var cfg Config
	if err := document.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if field, err := validateConfig(&cfg); err != nil {
		node := configFieldNode(&document, field)
		line := 1
		if node.Line > 0 {
			line = sourceLines[node.Line-1]
		}
		return nil, fmt.Errorf("line %d (%s): %w", line, field, err)
	}

	return &cfg, nil
}

// interpolateEnv replaces ${VAR} and ${VAR:-default} with environment values.
func interpolateEnv(input string) (string, []int) {
	var expanded strings.Builder
	sourceLines := []int{1}
	line := 1
	previousCR := false
	writeText := func(text string, literal bool) {
		expanded.WriteString(text)
		for i, ch := range text {
			if strings.ContainsRune("\r\n\u0085\u2028\u2029", ch) {
				if literal && (ch != '\n' || i == 0 || text[i-1] != '\r') {
					line++
				}
				if ch != '\n' || !previousCR {
					sourceLines = append(sourceLines, line)
				} else if literal {
					sourceLines[len(sourceLines)-1] = line
				}
			}
			previousCR = ch == '\r'
		}
	}
	end := 0
	for _, match := range envVarPattern.FindAllStringSubmatchIndex(input, -1) {
		writeText(input[end:match[0]], true)
		value, ok := os.LookupEnv(input[match[2]:match[3]])
		if !ok && match[4] >= 0 {
			value = input[match[4]:match[5]]
		}
		// Newlines supplied by the environment belong to the placeholder's source line.
		writeText(value, false)
		line += len(yamlLineBreaks.FindAllStringIndex(input[match[0]:match[1]], -1))
		end = match[1]
	}
	writeText(input[end:], true)
	return expanded.String(), sourceLines
}

// validateConfig checks that the config is semantically valid.
func validateConfig(cfg *Config) (string, error) {
	if len(cfg.Drills) == 0 {
		return "drills", fmt.Errorf("config: at least one drill must be defined")
	}

	seen := make(map[string]struct{})
	for i, drill := range cfg.Drills {
		field := fmt.Sprintf("drills[%d]", i)
		if drill.Name == "" {
			return field + ".name", fmt.Errorf("config: drill[%d] must have a name", i)
		}
		if _, exists := seen[drill.Name]; exists {
			return field + ".name", fmt.Errorf("config: duplicate drill name %q", drill.Name)
		}
		seen[drill.Name] = struct{}{}

		if drill.Provider == "" {
			return field + ".provider", fmt.Errorf("config: drill %q must specify a provider", drill.Name)
		}

		tools, ok := supportedBackupTools[drill.Provider]
		if !ok {
			return field + ".provider", fmt.Errorf("config: drill %q has unknown provider %q", drill.Name, drill.Provider)
		}

		if drill.Backup.Tool == "" {
			return field + ".backup.tool", fmt.Errorf("config: drill %q must specify backup.tool", drill.Name)
		}
		if _, ok := tools[drill.Backup.Tool]; !ok {
			return field + ".backup.tool", fmt.Errorf("config: drill %q has unsupported backup.tool %q for provider %q", drill.Name, drill.Backup.Tool, drill.Provider)
		}
		if drill.Backup.Source == "" && drill.Backup.Repo.Type == "" {
			return field + ".backup.source", fmt.Errorf("config: drill %q must specify backup.source or backup.repo.type", drill.Name)
		}
		if drill.Backup.Repo.Type != "" {
			if _, ok := supportedRepoTypes[drill.Backup.Repo.Type]; !ok {
				return field + ".backup.repo.type", fmt.Errorf("config: drill %q has unsupported backup.repo.type %q", drill.Name, drill.Backup.Repo.Type)
			}
			if drill.Backup.Repo.Bucket == "" {
				return field + ".backup.repo.bucket", fmt.Errorf("config: drill %q must specify backup.repo.bucket when backup.repo.type is set", drill.Name)
			}
			if drill.Backup.Repo.Prefix == "" {
				return field + ".backup.repo.prefix", fmt.Errorf("config: drill %q must specify backup.repo.prefix when backup.repo.type is set", drill.Name)
			}
		}

		if drill.Restore.Container.Image == "" {
			return field + ".restore.container.image", fmt.Errorf("config: drill %q must specify restore.container.image", drill.Name)
		}

		if drill.Restore.Timeout != "" {
			if _, err := time.ParseDuration(drill.Restore.Timeout); err != nil {
				return field + ".restore.timeout", fmt.Errorf("config: drill %q has invalid timeout %q: %w", drill.Name, drill.Restore.Timeout, err)
			}
		}

		if checkField, err := validateChecks(drill.Name, drill.Provider, drill.Validate); err != nil {
			return field + "." + checkField, err
		}
		if alertField, err := validateAlerts(drill.Name, drill.Alerts); err != nil {
			return field + "." + alertField, err
		}
	}

	return validateReporting(cfg.Reporting)
}

// validateChecks validates the check definitions for a drill.
func validateChecks(drillName, provider string, checks []Check) (string, error) {
	providerChecks := supportedCheckTypesByProvider[provider]
	for i, check := range checks {
		field := fmt.Sprintf("checks[%d]", i)
		if check.Name == "" {
			return field + ".name", fmt.Errorf("config: drill %q check[%d] must have a name", drillName, i)
		}

		if _, ok := supportedCheckTypes[check.Type]; !ok {
			return field + ".type", fmt.Errorf("config: drill %q check %q has unknown type %q", drillName, check.Name, check.Type)
		}

		if _, ok := providerChecks[check.Type]; !ok {
			return field + ".type", fmt.Errorf("config: drill %q check %q has unsupported type %q for provider %q", drillName, check.Name, check.Type, provider)
		}

		if _, ok := sqlBackedCheckTypes[check.Type]; ok && check.SQL == "" {
			return field + ".sql", fmt.Errorf("config: drill %q check %q of type %q must have sql", drillName, check.Name, check.Type)
		}

		if check.Type == "key_sample" && len(check.Keys) == 0 {
			return field + ".keys", fmt.Errorf("config: drill %q check %q of type 'key_sample' must have keys", drillName, check.Name)
		}

		if _, ok := keyBackedCheckTypes[check.Type]; ok && check.Key == "" {
			return field + ".key", fmt.Errorf("config: drill %q check %q of type %q must have key", drillName, check.Name, check.Type)
		}

		if check.Expect == "" {
			return field + ".expect", fmt.Errorf("config: drill %q check %q must have an expect expression", drillName, check.Name)
		}
	}
	return "", nil
}

func validateAlerts(drillName string, alerts []AlertSpec) (string, error) {
	for i, alert := range alerts {
		field := fmt.Sprintf("alerts[%d]", i)
		if alert.Type == "" {
			return field + ".type", fmt.Errorf("config: drill %q alert[%d] must have a type", drillName, i)
		}
		if _, ok := supportedAlertTypes[alert.Type]; !ok {
			return field + ".type", fmt.Errorf("config: drill %q alert[%d] has unsupported type %q", drillName, i, alert.Type)
		}
		if _, ok := supportedAlertConditions[alert.On]; !ok {
			return field + ".on", fmt.Errorf("config: drill %q alert[%d] has unsupported on %q (use always or failure)", drillName, i, alert.On)
		}
		switch alert.Type {
		case "webhook", "slack":
			if alert.URL == "" && alert.Endpoint == "" {
				return field + ".url", fmt.Errorf("config: drill %q %s alert[%d] must specify url or endpoint", drillName, alert.Type, i)
			}
		}
		for key := range alert.Headers {
			if strings.TrimSpace(key) == "" {
				return field + ".headers", fmt.Errorf("config: drill %q alert[%d] has an empty header name", drillName, i)
			}
		}
	}
	return "", nil
}

func validateReporting(reporting ReportConfig) (string, error) {
	for i, format := range reporting.Format {
		if _, ok := supportedReportFormats[format]; !ok {
			return fmt.Sprintf("reporting.format[%d]", i), fmt.Errorf("config: reporting.format has unsupported value %q", format)
		}
	}
	if reporting.Retention != "" {
		if _, err := reporting.RetentionDuration(); err != nil {
			return "reporting.retention", fmt.Errorf("config: reporting.retention %q: %w", reporting.Retention, err)
		}
	}
	return "", nil
}

// RetentionDuration returns the reporting retention window.
func (r ReportConfig) RetentionDuration() (time.Duration, error) {
	retention := strings.TrimSpace(r.Retention)
	if retention == "" {
		return defaultReportRetention, nil
	}
	if strings.HasSuffix(retention, "d") {
		days, err := strconv.ParseInt(strings.TrimSuffix(retention, "d"), 10, 64)
		if err != nil || days <= 0 {
			return 0, fmt.Errorf("must be a positive day count or duration")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	dur, err := time.ParseDuration(retention)
	if err != nil {
		return 0, err
	}
	if dur <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	return dur, nil
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

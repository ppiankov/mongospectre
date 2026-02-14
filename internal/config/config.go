package config

import (
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config holds all mongospectre configuration.
type Config struct {
	URI        string     `yaml:"uri"`
	Database   string     `yaml:"database"`
	Thresholds Thresholds `yaml:"thresholds"`
	Exclude    Exclude    `yaml:"exclude"`
	Defaults   Defaults   `yaml:"defaults"`
}

// Thresholds control detection sensitivity.
type Thresholds struct {
	OversizedDocs  int64 `yaml:"oversized_docs"`   // doc count to flag as oversized
	IndexUsageDays int   `yaml:"index_usage_days"` // days of zero ops to flag unused
}

// Exclude lists collections and databases to skip.
type Exclude struct {
	Collections []string `yaml:"collections"`
	Databases   []string `yaml:"databases"`
}

// Defaults holds default CLI flag values.
type Defaults struct {
	Format  string `yaml:"format"`
	Verbose bool   `yaml:"verbose"`
	Timeout string `yaml:"timeout"` // parsed as time.Duration
}

// DefaultConfig returns the built-in defaults.
func DefaultConfig() Config {
	return Config{
		Thresholds: Thresholds{
			OversizedDocs:  1_000_000,
			IndexUsageDays: 30,
		},
		Defaults: Defaults{
			Format:  "text",
			Timeout: "30s",
		},
	}
}

// Load reads configuration from .mongospectre.yml in the given directory,
// falling back to ~/.mongospectre.yml. Returns DefaultConfig if no file found.
func Load(dir string) (Config, error) {
	cfg := DefaultConfig()

	// Try CWD first, then home directory.
	paths := []string{filepath.Join(dir, ".mongospectre.yml")}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".mongospectre.yml"))
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // file not found, try next
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
		return cfg, nil
	}

	return cfg, nil
}

// TimeoutDuration parses the Defaults.Timeout string as a time.Duration.
// Returns 30s if parsing fails.
func (c *Config) TimeoutDuration() time.Duration {
	if c.Defaults.Timeout == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(c.Defaults.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

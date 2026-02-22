package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Thresholds.OversizedDocs != 1_000_000 {
		t.Errorf("oversized_docs = %d, want 1000000", cfg.Thresholds.OversizedDocs)
	}
	if cfg.Thresholds.IndexUsageDays != 30 {
		t.Errorf("index_usage_days = %d, want 30", cfg.Thresholds.IndexUsageDays)
	}
	if cfg.Defaults.Format != "text" {
		t.Errorf("format = %s, want text", cfg.Defaults.Format)
	}
	if cfg.Defaults.Timeout != "30s" {
		t.Errorf("timeout = %s, want 30s", cfg.Defaults.Timeout)
	}
}

func TestLoad_NoFile(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Should return defaults
	if cfg.Thresholds.OversizedDocs != 1_000_000 {
		t.Errorf("expected default oversized_docs, got %d", cfg.Thresholds.OversizedDocs)
	}
}

func TestLoad_FromDir(t *testing.T) {
	dir := t.TempDir()
	content := `
uri: mongodb://localhost:27017
database: myapp
thresholds:
  oversized_docs: 500000
  index_usage_days: 14
exclude:
  collections:
    - "migrations"
    - "system.*"
  databases:
    - "local"
defaults:
  format: json
  verbose: true
  timeout: 60s
notifications:
  - type: slack
    webhook_url: ${SLACK_WEBHOOK_URL}
    on: [new_high, new_medium]
  - type: webhook
    url: https://example.com/alerts
    method: POST
    headers:
      Authorization: "Bearer ${ALERT_TOKEN}"
    on: [new_high]
  - type: email
    smtp_host: smtp.example.com
    smtp_port: 587
    from: alerts@example.com
    to: ["team@example.com"]
    on: [resolved]
`
	if err := os.WriteFile(filepath.Join(dir, ".mongospectre.yml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.URI != "mongodb://localhost:27017" {
		t.Errorf("uri = %s", cfg.URI)
	}
	if cfg.Database != "myapp" {
		t.Errorf("database = %s", cfg.Database)
	}
	if cfg.Thresholds.OversizedDocs != 500000 {
		t.Errorf("oversized_docs = %d", cfg.Thresholds.OversizedDocs)
	}
	if cfg.Thresholds.IndexUsageDays != 14 {
		t.Errorf("index_usage_days = %d", cfg.Thresholds.IndexUsageDays)
	}
	if len(cfg.Exclude.Collections) != 2 {
		t.Errorf("exclude.collections = %v", cfg.Exclude.Collections)
	}
	if len(cfg.Exclude.Databases) != 1 {
		t.Errorf("exclude.databases = %v", cfg.Exclude.Databases)
	}
	if cfg.Defaults.Format != "json" {
		t.Errorf("format = %s", cfg.Defaults.Format)
	}
	if !cfg.Defaults.Verbose {
		t.Error("verbose should be true")
	}
	if cfg.Defaults.Timeout != "60s" {
		t.Errorf("timeout = %s", cfg.Defaults.Timeout)
	}
	if len(cfg.Notifications) != 3 {
		t.Fatalf("notifications = %d, want 3", len(cfg.Notifications))
	}
	if cfg.Notifications[0].Type != "slack" || cfg.Notifications[0].WebhookURL == "" {
		t.Errorf("unexpected slack notification: %+v", cfg.Notifications[0])
	}
	if cfg.Notifications[1].Type != "webhook" || cfg.Notifications[1].Headers["Authorization"] == "" {
		t.Errorf("unexpected webhook notification: %+v", cfg.Notifications[1])
	}
	if cfg.Notifications[2].Type != "email" || len(cfg.Notifications[2].To) != 1 {
		t.Errorf("unexpected email notification: %+v", cfg.Notifications[2])
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".mongospectre.yml"), []byte(":::invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestTimeoutDuration(t *testing.T) {
	tests := []struct {
		timeout string
		want    time.Duration
	}{
		{"30s", 30 * time.Second},
		{"2m", 2 * time.Minute},
		{"", 30 * time.Second},
		{"invalid", 30 * time.Second},
	}
	for _, tt := range tests {
		cfg := Config{Defaults: Defaults{Timeout: tt.timeout}}
		got := cfg.TimeoutDuration()
		if got != tt.want {
			t.Errorf("TimeoutDuration(%q) = %v, want %v", tt.timeout, got, tt.want)
		}
	}
}

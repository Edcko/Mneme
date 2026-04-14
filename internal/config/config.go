// Package config provides structured configuration for Engram's remote sync feature.
//
// Configuration is loaded with the following precedence (highest wins):
//
//  1. Environment variables (ENGRAM_SYNC_SERVER, ENGRAM_SYNC_API_KEY, etc.)
//  2. YAML config file (~/.engram/config.yaml by default)
//  3. Built-in defaults
//
// This package does NOT modify existing configuration in store or cmd packages.
// It is a standalone addition for the upcoming remote sync subsystem.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// ─── Defaults ────────────────────────────────────────────────────────────────

const (
	DefaultSyncInterval = 5 * time.Minute
)

// ─── Types ───────────────────────────────────────────────────────────────────

// Config is the top-level configuration for Engram's remote sync.
type Config struct {
	Sync SyncConfig `yaml:"sync"`
}

// SyncConfig holds all sync-related settings.
type SyncConfig struct {
	// DefaultRemote is the name of the remote to use when no specific remote is given.
	DefaultRemote string `yaml:"default_remote"`

	// SyncInterval controls how often automatic sync runs. Defaults to 5 minutes.
	SyncInterval time.Duration `yaml:"sync_interval"`

	// Remotes maps named remote server configurations.
	// The key is the user-chosen name (e.g. "production", "staging").
	Remotes map[string]RemoteConfig `yaml:"remotes"`
}

// RemoteConfig holds connection details for a single remote Engram server.
type RemoteConfig struct {
	// ServerURL is the base URL of the remote Engram server (e.g. "https://engram.example.com").
	ServerURL string `yaml:"server_url"`

	// APIKey is the authentication key for the remote server.
	APIKey string `yaml:"api_key"`

	// ProjectMappings maps local project names to remote project identifiers.
	// Example: {"frontend": "my-org/web-app"}
	ProjectMappings map[string]string `yaml:"project_mappings"`

	// TLS holds optional TLS/mTLS settings for this remote.
	TLS TLSSettings `yaml:"tls"`
}

// TLSSettings holds TLS configuration for connecting to a remote server.
type TLSSettings struct {
	// CACertPath is the path to a custom CA certificate to trust.
	CACertPath string `yaml:"ca_cert_path"`

	// CertPath is the path to a client certificate for mTLS.
	CertPath string `yaml:"cert_path"`

	// KeyPath is the path to a client key for mTLS.
	KeyPath string `yaml:"key_path"`

	// InsecureSkipVerify disables TLS certificate verification.
	// Should only be used in development/testing.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}

// ─── Load Options ────────────────────────────────────────────────────────────

// loadOption configures the behavior of Load.
type loadOption struct {
	configPath string
}

// LoadOption is a functional option for Load.
type LoadOption func(*loadOption)

// WithPath overrides the config file path used by Load.
func WithPath(p string) LoadOption {
	return func(o *loadOption) {
		o.configPath = p
	}
}

// ─── Load ────────────────────────────────────────────────────────────────────

// Load reads configuration from: env vars → YAML file → defaults.
//
// If the YAML file does not exist, Load returns defaults (no error).
// If the YAML file exists but is malformed, Load returns an error.
// Environment variables always override file values.
func Load(opts ...LoadOption) (*Config, error) {
	o := &loadOption{
		configPath: DefaultConfigPath(),
	}
	for _, opt := range opts {
		opt(o)
	}

	cfg := &Config{}
	applyDefaults(cfg)

	// Layer 1: YAML file
	if _, err := os.Stat(o.configPath); err == nil {
		data, err := os.ReadFile(o.configPath)
		if err != nil {
			return nil, fmt.Errorf("config: read %s: %w", o.configPath, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("config: parse %s: %w", o.configPath, err)
		}
	}

	// Layer 2: Env var overrides
	applyEnvOverrides(cfg)

	// Ensure defaults are still set for zero fields after merge
	ensureDefaults(cfg)

	return cfg, nil
}

// ─── Save ────────────────────────────────────────────────────────────────────

// Save writes the configuration to the given file path as YAML.
// It creates parent directories as needed.
func Save(cfg *Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("config: create dir %s: %w", dir, err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}

	return nil
}

// ─── Validate ────────────────────────────────────────────────────────────────

// Validate checks the configuration for required fields and consistency.
// An empty config (no remotes) is valid — it just means sync is not configured.
func (c *Config) Validate() error {
	if c.Sync.DefaultRemote != "" {
		if _, ok := c.Sync.Remotes[c.Sync.DefaultRemote]; !ok {
			return fmt.Errorf("config: default_remote %q not found in remotes", c.Sync.DefaultRemote)
		}
	}

	for name, remote := range c.Sync.Remotes {
		if remote.ServerURL == "" {
			return fmt.Errorf("config: remote %q: server_url is required", name)
		}
		parsed, err := url.Parse(remote.ServerURL)
		if err != nil {
			return fmt.Errorf("config: remote %q: invalid server_url %q: %w", name, remote.ServerURL, err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("config: remote %q: server_url must have http or https scheme, got %q", name, remote.ServerURL)
		}
		if parsed.Host == "" {
			return fmt.Errorf("config: remote %q: server_url must have a host, got %q", name, remote.ServerURL)
		}
		if remote.APIKey == "" {
			return fmt.Errorf("config: remote %q: api_key is required", name)
		}
	}

	return nil
}

// ─── Default Path ────────────────────────────────────────────────────────────

// DefaultConfigPath returns the default configuration file path: ~/.engram/config.yaml
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback: use the path that the rest of Engram uses
		return filepath.Join(".engram", "config.yaml")
	}
	return filepath.Join(home, ".engram", "config.yaml")
}

// ─── Internal Helpers ────────────────────────────────────────────────────────

func applyDefaults(cfg *Config) {
	cfg.Sync.SyncInterval = DefaultSyncInterval
}

func ensureDefaults(cfg *Config) {
	if cfg.Sync.SyncInterval == 0 {
		cfg.Sync.SyncInterval = DefaultSyncInterval
	}
	if cfg.Sync.Remotes == nil {
		cfg.Sync.Remotes = make(map[string]RemoteConfig)
	}
}

func applyEnvOverrides(cfg *Config) {
	serverURL := os.Getenv("ENGRAM_SYNC_SERVER")
	apiKey := os.Getenv("ENGRAM_SYNC_API_KEY")
	intervalStr := os.Getenv("ENGRAM_SYNC_INTERVAL")

	if serverURL != "" || apiKey != "" {
		// Env vars create/update the "default" remote
		remote := cfg.Sync.Remotes["default"]
		if serverURL != "" {
			remote.ServerURL = serverURL
		}
		if apiKey != "" {
			remote.APIKey = apiKey
		}
		if cfg.Sync.Remotes == nil {
			cfg.Sync.Remotes = make(map[string]RemoteConfig)
		}
		cfg.Sync.Remotes["default"] = remote
		cfg.Sync.DefaultRemote = "default"
	}

	if intervalStr != "" {
		if d, err := time.ParseDuration(intervalStr); err == nil {
			cfg.Sync.SyncInterval = d
		}
	}
}

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── Load: YAML file precedence ──────────────────────────────────────────────

func TestLoad_ReadsYAMLConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yamlContent := `
sync:
  default_remote: production
  sync_interval: 10m
  remotes:
    production:
      server_url: https://engram.example.com
      api_key: key-prod-123
      project_mappings:
        local: my-org/prod-project
      tls:
        insecure_skip_verify: true
    staging:
      server_url: https://staging.engram.example.com
      api_key: key-staging-456
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	cfg, err := Load(WithPath(cfgPath))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Sync.DefaultRemote != "production" {
		t.Errorf("DefaultRemote = %q, want %q", cfg.Sync.DefaultRemote, "production")
	}
	if cfg.Sync.SyncInterval != 10*time.Minute {
		t.Errorf("SyncInterval = %v, want %v", cfg.Sync.SyncInterval, 10*time.Minute)
	}

	prod, ok := cfg.Sync.Remotes["production"]
	if !ok {
		t.Fatal("missing 'production' remote")
	}
	if prod.ServerURL != "https://engram.example.com" {
		t.Errorf("ServerURL = %q, want %q", prod.ServerURL, "https://engram.example.com")
	}
	if prod.APIKey != "key-prod-123" {
		t.Errorf("APIKey = %q, want %q", prod.APIKey, "key-prod-123")
	}
	if prod.TLS.InsecureSkipVerify != true {
		t.Error("InsecureSkipVerify should be true")
	}
	if prod.ProjectMappings["local"] != "my-org/prod-project" {
		t.Errorf("ProjectMapping = %q, want %q", prod.ProjectMappings["local"], "my-org/prod-project")
	}

	staging, ok := cfg.Sync.Remotes["staging"]
	if !ok {
		t.Fatal("missing 'staging' remote")
	}
	if staging.ServerURL != "https://staging.engram.example.com" {
		t.Errorf("staging ServerURL = %q, want %q", staging.ServerURL, "https://staging.engram.example.com")
	}
}

func TestLoad_YAMLFileNotFound(t *testing.T) {
	cfg, err := Load(WithPath(filepath.Join(t.TempDir(), "nonexistent.yaml")))
	if err != nil {
		t.Fatalf("Load with missing file should not error, got: %v", err)
	}
	// Should return defaults
	if cfg.Sync.SyncInterval != DefaultSyncInterval {
		t.Errorf("SyncInterval = %v, want default %v", cfg.Sync.SyncInterval, DefaultSyncInterval)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(cfgPath, []byte(":\n  invalid: [yaml: content"), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	_, err := Load(WithPath(cfgPath))
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

// ─── Load: Env var override ──────────────────────────────────────────────────

func TestLoad_EnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yamlContent := `
sync:
  default_remote: yaml-remote
  sync_interval: 5m
  remotes:
    yaml-remote:
      server_url: https://yaml.example.com
      api_key: yaml-key
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	t.Setenv("ENGRAM_SYNC_SERVER", "https://env.example.com")
	t.Setenv("ENGRAM_SYNC_API_KEY", "env-key-override")
	t.Setenv("ENGRAM_SYNC_INTERVAL", "15m")

	cfg, err := Load(WithPath(cfgPath))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Env vars create a "default" remote override and take precedence
	if cfg.Sync.DefaultRemote != "default" {
		t.Errorf("DefaultRemote = %q, want %q (env should override)", cfg.Sync.DefaultRemote, "default")
	}
	if cfg.Sync.SyncInterval != 15*time.Minute {
		t.Errorf("SyncInterval = %v, want %v (env override)", cfg.Sync.SyncInterval, 15*time.Minute)
	}

	envRemote, ok := cfg.Sync.Remotes["default"]
	if !ok {
		t.Fatal("env vars should create a 'default' remote")
	}
	if envRemote.ServerURL != "https://env.example.com" {
		t.Errorf("ServerURL = %q, want %q (env override)", envRemote.ServerURL, "https://env.example.com")
	}
	if envRemote.APIKey != "env-key-override" {
		t.Errorf("APIKey = %q, want %q (env override)", envRemote.APIKey, "env-key-override")
	}
}

func TestLoad_EnvOnlyNoYAML(t *testing.T) {
	t.Setenv("ENGRAM_SYNC_SERVER", "https://only-env.example.com")
	t.Setenv("ENGRAM_SYNC_API_KEY", "env-only-key")

	cfg, err := Load(WithPath(filepath.Join(t.TempDir(), "missing.yaml")))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Sync.DefaultRemote != "default" {
		t.Errorf("DefaultRemote = %q, want %q", cfg.Sync.DefaultRemote, "default")
	}
	envRemote := cfg.Sync.Remotes["default"]
	if envRemote.ServerURL != "https://only-env.example.com" {
		t.Errorf("ServerURL = %q, want %q", envRemote.ServerURL, "https://only-env.example.com")
	}
}

// ─── Load: Defaults ──────────────────────────────────────────────────────────

func TestLoad_DefaultsWhenEmpty(t *testing.T) {
	cfg, err := Load(WithPath(filepath.Join(t.TempDir(), "missing.yaml")))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Sync.SyncInterval != DefaultSyncInterval {
		t.Errorf("SyncInterval = %v, want default %v", cfg.Sync.SyncInterval, DefaultSyncInterval)
	}
	if cfg.Sync.DefaultRemote != "" {
		t.Errorf("DefaultRemote = %q, want empty", cfg.Sync.DefaultRemote)
	}
	if len(cfg.Sync.Remotes) != 0 {
		t.Errorf("Remotes = %d entries, want 0", len(cfg.Sync.Remotes))
	}
}

// ─── Save ────────────────────────────────────────────────────────────────────

func TestSave_And_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	original := &Config{
		Sync: SyncConfig{
			DefaultRemote: "my-server",
			SyncInterval:  30 * time.Minute,
			Remotes: map[string]RemoteConfig{
				"my-server": {
					ServerURL: "https://engram.mycompany.com",
					APIKey:    "super-secret-key",
					ProjectMappings: map[string]string{
						"frontend": "company/web-app",
						"backend":  "company/api-server",
					},
					TLS: TLSSettings{
						CACertPath:         "/etc/ssl/myca.pem",
						InsecureSkipVerify: false,
					},
				},
			},
		},
	}

	if err := Save(original, cfgPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(WithPath(cfgPath))
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}

	if loaded.Sync.DefaultRemote != original.Sync.DefaultRemote {
		t.Errorf("DefaultRemote: got %q, want %q", loaded.Sync.DefaultRemote, original.Sync.DefaultRemote)
	}
	if loaded.Sync.SyncInterval != original.Sync.SyncInterval {
		t.Errorf("SyncInterval: got %v, want %v", loaded.Sync.SyncInterval, original.Sync.SyncInterval)
	}

	got := loaded.Sync.Remotes["my-server"]
	want := original.Sync.Remotes["my-server"]
	if got.ServerURL != want.ServerURL {
		t.Errorf("ServerURL: got %q, want %q", got.ServerURL, want.ServerURL)
	}
	if got.APIKey != want.APIKey {
		t.Errorf("APIKey: got %q, want %q", got.APIKey, want.APIKey)
	}
	if got.TLS.CACertPath != want.TLS.CACertPath {
		t.Errorf("CACertPath: got %q, want %q", got.TLS.CACertPath, want.TLS.CACertPath)
	}
	if got.ProjectMappings["frontend"] != want.ProjectMappings["frontend"] {
		t.Errorf("ProjectMapping frontend: got %q, want %q", got.ProjectMappings["frontend"], want.ProjectMappings["frontend"])
	}
}

func TestSave_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "deep", "nested", "config.yaml")

	cfg := &Config{Sync: SyncConfig{SyncInterval: DefaultSyncInterval}}
	if err := Save(cfg, cfgPath); err != nil {
		t.Fatalf("Save with nested dirs: %v", err)
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}
}

// ─── Validation ──────────────────────────────────────────────────────────────

func TestValidate_RemoteWithEmptyURL(t *testing.T) {
	cfg := &Config{
		Sync: SyncConfig{
			DefaultRemote: "broken",
			Remotes: map[string]RemoteConfig{
				"broken": {
					ServerURL: "",
					APIKey:    "some-key",
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for empty URL, got nil")
	}
}

func TestValidate_RemoteWithInvalidURL(t *testing.T) {
	cfg := &Config{
		Sync: SyncConfig{
			DefaultRemote: "bad",
			Remotes: map[string]RemoteConfig{
				"bad": {
					ServerURL: "not-a-url",
					APIKey:    "some-key",
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid URL, got nil")
	}
}

func TestValidate_RemoteWithEmptyAPIKey(t *testing.T) {
	cfg := &Config{
		Sync: SyncConfig{
			DefaultRemote: "nokey",
			Remotes: map[string]RemoteConfig{
				"nokey": {
					ServerURL: "https://engram.example.com",
					APIKey:    "",
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for empty API key, got nil")
	}
}

func TestValidate_DefaultRemoteNotInRemotes(t *testing.T) {
	cfg := &Config{
		Sync: SyncConfig{
			DefaultRemote: "ghost",
			Remotes: map[string]RemoteConfig{
				"real": {
					ServerURL: "https://engram.example.com",
					APIKey:    "key-123",
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for missing default remote, got nil")
	}
}

func TestValidate_EmptyConfig(t *testing.T) {
	cfg := &Config{}
	// Empty config is valid — no remotes configured is fine
	if err := cfg.Validate(); err != nil {
		t.Errorf("empty config should be valid, got: %v", err)
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		Sync: SyncConfig{
			DefaultRemote: "prod",
			Remotes: map[string]RemoteConfig{
				"prod": {
					ServerURL: "https://engram.example.com",
					APIKey:    "valid-key",
				},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config should pass validation, got: %v", err)
	}
}

// ─── Multiple Remotes ────────────────────────────────────────────────────────

func TestLoad_MultipleRemotes(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yamlContent := `
sync:
  default_remote: work
  remotes:
    personal:
      server_url: https://personal.engram.dev
      api_key: pers-key
    work:
      server_url: https://work.engram.corp
      api_key: work-key
      project_mappings:
        myapp: acme/myapp
    client-a:
      server_url: https://client-a.engram.host
      api_key: client-a-key
      tls:
        ca_cert_path: /certs/client-a-ca.pem
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	cfg, err := Load(WithPath(cfgPath))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Sync.Remotes) != 3 {
		t.Fatalf("expected 3 remotes, got %d", len(cfg.Sync.Remotes))
	}
	if cfg.Sync.DefaultRemote != "work" {
		t.Errorf("DefaultRemote = %q, want %q", cfg.Sync.DefaultRemote, "work")
	}

	clientA := cfg.Sync.Remotes["client-a"]
	if clientA.TLS.CACertPath != "/certs/client-a-ca.pem" {
		t.Errorf("CACertPath = %q, want %q", clientA.TLS.CACertPath, "/certs/client-a-ca.pem")
	}

	work := cfg.Sync.Remotes["work"]
	if work.ProjectMappings["myapp"] != "acme/myapp" {
		t.Errorf("ProjectMapping = %q, want %q", work.ProjectMappings["myapp"], "acme/myapp")
	}
}

// ─── Default Path Resolution ─────────────────────────────────────────────────

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	if path == "" {
		t.Fatal("DefaultConfigPath returned empty string")
	}
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("expected filename config.yaml, got %q", filepath.Base(path))
	}
	// Must be under ~/.engram
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot resolve home dir")
	}
	expectedDir := filepath.Join(home, ".engram")
	if filepath.Dir(path) != expectedDir {
		t.Errorf("path = %q, expected dir %q", path, expectedDir)
	}
}

// ─── TLS Settings ────────────────────────────────────────────────────────────

func TestLoad_TLSSettings(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	yamlContent := `
sync:
  remotes:
    secure:
      server_url: https://secure.engram.io
      api_key: secure-key
      tls:
        ca_cert_path: /etc/ssl/custom-ca.pem
        cert_path: /etc/ssl/client-cert.pem
        key_path: /etc/ssl/client-key.pem
        insecure_skip_verify: false
`
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	cfg, err := Load(WithPath(cfgPath))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	remote := cfg.Sync.Remotes["secure"]
	if remote.TLS.CACertPath != "/etc/ssl/custom-ca.pem" {
		t.Errorf("CACertPath = %q, want custom CA", remote.TLS.CACertPath)
	}
	if remote.TLS.CertPath != "/etc/ssl/client-cert.pem" {
		t.Errorf("CertPath = %q, want client cert", remote.TLS.CertPath)
	}
	if remote.TLS.KeyPath != "/etc/ssl/client-key.pem" {
		t.Errorf("KeyPath = %q, want client key", remote.TLS.KeyPath)
	}
	if remote.TLS.InsecureSkipVerify != false {
		t.Error("InsecureSkipVerify should be false")
	}
}

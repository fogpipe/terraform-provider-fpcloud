package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCLIConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("api_url: https://api.example.com\napi_key: fp-test-key\ncurrent_project: demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FPCLOUD_STATE_DIR", "")
	t.Setenv("FPCLOUD_CONFIG_DIR", dir)

	cfg := loadCLIConfig()
	if cfg.APIKey != "fp-test-key" {
		t.Errorf("api_key = %q, want fp-test-key", cfg.APIKey)
	}
	if cfg.APIURL != "https://api.example.com" {
		t.Errorf("api_url = %q, want https://api.example.com", cfg.APIURL)
	}
}

func TestLoadCLIConfigMissingFileIsZero(t *testing.T) {
	t.Setenv("FPCLOUD_STATE_DIR", "")
	t.Setenv("FPCLOUD_CONFIG_DIR", t.TempDir()) // dir exists, no config.yaml
	if cfg := loadCLIConfig(); cfg.APIKey != "" || cfg.APIURL != "" {
		t.Errorf("missing config should be zero, got %+v", cfg)
	}
}

// FPCLOUD_STATE_DIR relocates the CLI's whole state dir, config.yaml included, so
// a fully repo-local login is still the provider's default credential source.
func TestLoadCLIConfigHonorsStateDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("api_key: fp-state-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FPCLOUD_CONFIG_DIR", "")
	t.Setenv("FPCLOUD_STATE_DIR", dir)

	if cfg := loadCLIConfig(); cfg.APIKey != "fp-state-key" {
		t.Errorf("api_key = %q, want fp-state-key", cfg.APIKey)
	}
}

// FPCLOUD_CONFIG_DIR scopes config.yaml alone, so it wins over FPCLOUD_STATE_DIR
// when both are set — same precedence the CLI applies.
func TestLoadCLIConfigConfigDirWinsOverStateDir(t *testing.T) {
	configDir, stateDir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("api_key: fp-config-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "config.yaml"), []byte("api_key: fp-state-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FPCLOUD_CONFIG_DIR", configDir)
	t.Setenv("FPCLOUD_STATE_DIR", stateDir)

	if cfg := loadCLIConfig(); cfg.APIKey != "fp-config-key" {
		t.Errorf("api_key = %q, want fp-config-key", cfg.APIKey)
	}
}

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
	t.Setenv("FPCLOUD_CONFIG_DIR", t.TempDir()) // dir exists, no config.yaml
	if cfg := loadCLIConfig(); cfg.APIKey != "" || cfg.APIURL != "" {
		t.Errorf("missing config should be zero, got %+v", cfg)
	}
}

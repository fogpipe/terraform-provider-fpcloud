package provider

import (
	"os"
	"path/filepath"
	"testing"
)

// checkout builds a directory tree holding its own fpcloud state and chdirs
// into a subdirectory of it, planting HOME elsewhere so the test never reads
// the config of whoever is running it.
func checkout(t *testing.T, body string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "repo")
	deep := filepath.Join(repo, "stacks", "prod")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		if err := os.MkdirAll(filepath.Join(repo, ".fpcloud"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, ".fpcloud", "config.yaml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Chdir(deep)
	return root
}

// The provider finds the CLI's config the way the CLI does — by walking up from
// the working directory — so a stack applied from inside a checkout that keeps
// its own fpcloud state inherits that state's credential.
func TestLoadCLIConfig(t *testing.T) {
	checkout(t, "api_url: https://api.example.com\napi_key: fp-test-key\ncurrent_project: demo\n")

	cfg := loadCLIConfig()
	if cfg.APIKey != "fp-test-key" {
		t.Errorf("api_key = %q, want fp-test-key", cfg.APIKey)
	}
	if cfg.APIURL != "https://api.example.com" {
		t.Errorf("api_url = %q, want https://api.example.com", cfg.APIURL)
	}
}

func TestLoadCLIConfigMissingFileIsZero(t *testing.T) {
	checkout(t, "")
	if cfg := loadCLIConfig(); cfg.APIKey != "" || cfg.APIURL != "" {
		t.Errorf("missing config should be zero, got %+v", cfg)
	}
}

// With nothing local, the global login is still the provider's default
// credential source — the fallback the walk ends at.
func TestLoadCLIConfigFallsBackToHome(t *testing.T) {
	root := checkout(t, "")
	home := filepath.Join(root, "home", ".fpcloud")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("api_key: fp-home-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if cfg := loadCLIConfig(); cfg.APIKey != "fp-home-key" {
		t.Errorf("api_key = %q, want fp-home-key", cfg.APIKey)
	}
}

// The nearest one wins, so a stack directory can carry a credential of its own
// inside a checkout that already carries one.
func TestLoadCLIConfigNearestWins(t *testing.T) {
	checkout(t, "api_key: fp-repo-key\n")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(wd, ".fpcloud"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wd, ".fpcloud", "config.yaml"), []byte("api_key: fp-stack-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if cfg := loadCLIConfig(); cfg.APIKey != "fp-stack-key" {
		t.Errorf("api_key = %q, want fp-stack-key", cfg.APIKey)
	}
}

package provider

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// cliConfig mirrors the subset of the fpcloud CLI's config.yaml that
// the provider can reuse as credentials — the AWS/GCP model where the CLI login
// doubles as the provider's default credential source.
type cliConfig struct {
	APIURL string `yaml:"api_url"`
	APIKey string `yaml:"api_key"`
}

// loadCLIConfig reads the fpcloud CLI config, finding it exactly the way the CLI
// does — the nearest .fpcloud/ holding one at or above the working directory,
// else ~/.fpcloud (fogpipe/cloud-workspace#768). So a stack applied from inside
// a checkout that keeps its own fpcloud state inherits that state's credential,
// which is the same directory rule `tofu` itself is run under. A
// missing/unreadable file yields a zero config, never an error — it is a
// best-effort last resort behind the block and env var.
func loadCLIConfig() cliConfig {
	path := cliConfigPath()
	if path == "" {
		return cliConfig{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cliConfig{}
	}
	var cfg cliConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cliConfig{}
	}
	return cfg
}

// cliConfigPath mirrors the CLI's own walk: the nearest .fpcloud/config.yaml at
// or above the working directory, else the one in ~/.fpcloud. Returns "" if
// there is no local one and the home directory cannot be determined.
func cliConfigPath() string {
	if dir, err := os.Getwd(); err == nil {
		for {
			p := filepath.Join(dir, ".fpcloud", "config.yaml")
			if _, err := os.Stat(p); err == nil {
				return p
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".fpcloud", "config.yaml")
}

// cliOIDCToken shells out to `fpcloud get-token` and returns the OIDC
// id-token the CLI login caches — transparently refreshed by the CLI from its
// stored refresh token. This is the gcloud-ADC model: the provider can't refresh
// itself (the OAuth client is baked into the fpcloud binary, not here), so it
// delegates to the CLI, exactly like a kubectl exec credential plugin. The API
// accepts this id-token as a bearer credential. Best-effort: a missing binary,
// no login, or a stale refresh token yields "" (never an error) so it stays a
// silent last resort behind the block, env var, and config.yaml key.
func cliOIDCToken() string {
	out, err := exec.Command("fpcloud", "get-token").Output()
	if err != nil {
		return ""
	}
	var cred struct {
		Status struct {
			Token string `json:"token"`
		} `json:"status"`
	}
	if err := json.Unmarshal(out, &cred); err != nil {
		return ""
	}
	return cred.Status.Token
}

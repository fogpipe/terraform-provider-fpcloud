package provider

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// cliConfig mirrors the subset of the fpcloud CLI's ~/.fpcloud/config.yaml that
// the provider can reuse as credentials — the AWS/GCP model where the CLI login
// doubles as the provider's default credential source.
type cliConfig struct {
	APIURL string `yaml:"api_url"`
	APIKey string `yaml:"api_key"`
}

// loadCLIConfig reads the fpcloud CLI config, honouring FPCLOUD_CONFIG_DIR the
// same way the CLI does (so a direnv-scoped per-project config is picked up too),
// and falling back to ~/.fpcloud. A missing/unreadable file yields a zero config,
// never an error — it is a best-effort last resort behind the block and env var.
func loadCLIConfig() cliConfig {
	dir := os.Getenv("FPCLOUD_CONFIG_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return cliConfig{}
		}
		dir = filepath.Join(home, ".fpcloud")
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return cliConfig{}
	}
	var cfg cliConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cliConfig{}
	}
	return cfg
}

// cliOIDCToken shells out to `fpcloud get-token` and returns the Google OIDC
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

package provider

import (
	"os"
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

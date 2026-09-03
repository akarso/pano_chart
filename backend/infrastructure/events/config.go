package events

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// apiKeysConfig mirrors the api_keys section of config.yaml.
type apiKeysConfig struct {
	APIKeys struct {
		FinanceFlow []struct {
			Name string `yaml:"name"`
			Key  string `yaml:"key"`
		} `yaml:"financeflow"`
	} `yaml:"api_keys"`
}

// LoadFinanceFlowAPIKey reads the FinanceFlow API key from config.yaml.
// Falls back to the FINANCEFLOW_API_KEY environment variable.
func LoadFinanceFlowAPIKey(configPath string) (string, error) {
	// Try env var first (override)
	if key := os.Getenv("FINANCEFLOW_API_KEY"); key != "" {
		return key, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read config %s: %w", configPath, err)
	}

	var cfg apiKeysConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse config %s: %w", configPath, err)
	}

	if len(cfg.APIKeys.FinanceFlow) == 0 {
		return "", fmt.Errorf("no financeflow api key found in %s", configPath)
	}

	key := cfg.APIKeys.FinanceFlow[0].Key
	if key == "" {
		return "", fmt.Errorf("financeflow api key is empty in %s", configPath)
	}

	return key, nil
}

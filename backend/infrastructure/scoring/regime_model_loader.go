package scoring

import (
	"fmt"
	"os"
	"path/filepath"

	domainscoring "pano_chart/backend/domain/scoring"
)

// ResolveRegimeModelPath finds regime_model.yaml.
// Priority: explicit config path > $REGIME_MODEL_PATH > sibling of config.yaml >
// config/regime_model.yaml under the config directory > "config/regime_model.yaml".
func ResolveRegimeModelPath(configYAMLPath, configuredPath string) string {
	if configuredPath != "" {
		return configuredPath
	}
	if p := os.Getenv("REGIME_MODEL_PATH"); p != "" {
		return p
	}
	if configYAMLPath != "" {
		dir := filepath.Dir(configYAMLPath)
		candidate := filepath.Join(dir, "regime_model.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		candidate = filepath.Join(dir, "config", "regime_model.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "config/regime_model.yaml"
}

// LoadRegimeModelFile reads and parses a regime_model.yaml from disk.
func LoadRegimeModelFile(path string) (*domainscoring.Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read regime model %s: %w", path, err)
	}
	m, err := domainscoring.ParseRegimeModelYAML(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

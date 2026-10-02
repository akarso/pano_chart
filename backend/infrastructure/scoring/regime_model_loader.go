package scoring

import (
	"fmt"
	"os"
	"path/filepath"

	domainscoring "pano_chart/backend/domain/scoring"

	"gopkg.in/yaml.v3"
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

// YAML wire format for regime_model.yaml (kept out of domain).
type regimeModelYAML struct {
	Model       string                     `yaml:"model"`
	Features    []string                   `yaml:"features"`
	Weights     map[string]float64         `yaml:"weights"`
	Bias        float64                    `yaml:"bias"`
	Classes     map[string]classParamsYAML `yaml:"classes"`
	Placeholder bool                       `yaml:"placeholder"`
}

type classParamsYAML struct {
	Weights map[string]float64 `yaml:"weights"`
	Bias    float64            `yaml:"bias"`
}

// ParseRegimeModelYAML unmarshals deployment YAML into a domain Model and validates.
func ParseRegimeModelYAML(data []byte) (*domainscoring.Model, error) {
	var raw regimeModelYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse regime model: %w", err)
	}
	m := &domainscoring.Model{
		Type:        raw.Model,
		Features:    raw.Features,
		Weights:     raw.Weights,
		Bias:        raw.Bias,
		Placeholder: raw.Placeholder,
		Classes:     make(map[string]domainscoring.ClassParams, len(raw.Classes)),
	}
	for name, cp := range raw.Classes {
		m.Classes[name] = domainscoring.ClassParams{Weights: cp.Weights, Bias: cp.Bias}
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("invalid regime model: %w", err)
	}
	return m, nil
}

// LoadRegimeModelFile reads and parses a regime_model.yaml from disk.
func LoadRegimeModelFile(path string) (*domainscoring.Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read regime model %s: %w", path, err)
	}
	m, err := ParseRegimeModelYAML(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

package scoring

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// activeRegimeModel is an optional learned classifier for ScoreMarketTape /
// per-symbol hooks (PR-109). Nil means heuristic classification.
var (
	regimeModelMu sync.RWMutex
	regimeModel   *Model
)

// SetRegimeModel installs a process-wide learned regime model. Pass nil to clear.
// Not safe with t.Parallel() against other regime-model tests — use
// t.Cleanup(ClearRegimeModel).
func SetRegimeModel(m *Model) {
	regimeModelMu.Lock()
	regimeModel = m
	regimeModelMu.Unlock()
}

// ClearRegimeModel clears the process-wide learned regime model.
func ClearRegimeModel() {
	SetRegimeModel(nil)
}

// ActiveRegimeModel returns the installed learned model, or nil.
func ActiveRegimeModel() *Model {
	regimeModelMu.RLock()
	defer regimeModelMu.RUnlock()
	return regimeModel
}

// RegimeModelMode returns "heuristic" or "learned" from config (default heuristic).
// Unknown values are treated as heuristic; use ValidateRegimeModelMode at
// startup to fail closed on typos.
func RegimeModelMode() string {
	cfg := GetConfig()
	if cfg == nil {
		return "heuristic"
	}
	switch cfg.Scoring.RegimeModel {
	case "learned":
		return "learned"
	default:
		return "heuristic"
	}
}

// ValidateRegimeModelMode returns an error when scoring.regime_model is set to
// something other than heuristic|learned|"".
func ValidateRegimeModelMode() error {
	cfg := GetConfig()
	if cfg == nil {
		return nil
	}
	mode := cfg.Scoring.RegimeModel
	switch mode {
	case "", "heuristic", "learned":
		return nil
	default:
		return fmt.Errorf("invalid scoring.regime_model %q (want heuristic|learned)", mode)
	}
}

// RegimeModelPath resolves where to find regime_model.yaml.
// Priority: Scoring.RegimeModelPath in config > $REGIME_MODEL_PATH >
// sibling of config.yaml named regime_model.yaml > config/regime_model.yaml.
func RegimeModelPath() string {
	if cfg := GetConfig(); cfg != nil {
		if p := cfg.Scoring.RegimeModelPath; p != "" {
			return p
		}
	}
	if p := os.Getenv("REGIME_MODEL_PATH"); p != "" {
		return p
	}
	if cfgPath := ConfigPath(); cfgPath != "" {
		dir := filepath.Dir(cfgPath)
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

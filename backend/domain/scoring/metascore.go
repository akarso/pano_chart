package scoring

import (
	"fmt"
	"math"
	"os"
)

// SubscoreResult represents a normalized subscore with optional confidence and metadata.
type SubscoreResult struct {
	Value      float64            // normalized [0,1]
	Confidence float64            // normalized [0,1], defaults to 1.0
	Meta       map[string]float64 // optional breakdown data
}

func Clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// Subscore interface for independent subscores.
type Subscore interface {
	Name() string
	Compute(data interface{}, cfg interface{}) SubscoreResult // CandleSet type to be replaced
}

// CompositionMode defines how subscores are combined.
type CompositionMode int

const (
	WeightedAdditive CompositionMode = iota
	WeightedMultiplicative
	HybridStructural
)

// MetaConfig holds composition settings.
type MetaConfig struct {
	Mode                CompositionMode
	BaseWeights         map[string]float64
	UseConfidenceWeight bool
}

// TimeframeProfile holds per-timeframe configuration for MetaScorer.
type TimeframeProfile struct {
	Timeframe   string
	CandleCount int
	SubConfigs  map[string]interface{}
	MetaConfig  MetaConfig
}

// Recommended timeframe profile defaults
var DefaultTimeframeProfiles = map[string]TimeframeProfile{
	"1m": {
		Timeframe:   "1m",
		CandleCount: 120,
		SubConfigs:  map[string]interface{}{},
		MetaConfig:  MetaConfig{},
	},
	"5m": {
		Timeframe:   "5m",
		CandleCount: 100,
		SubConfigs:  map[string]interface{}{},
		MetaConfig:  MetaConfig{},
	},
	"15m": {
		Timeframe:   "15m",
		CandleCount: 120,
		SubConfigs:  map[string]interface{}{},
		MetaConfig:  MetaConfig{},
	},
	"1h": {
		Timeframe:   "1h",
		CandleCount: 100,
		SubConfigs:  map[string]interface{}{},
		MetaConfig:  MetaConfig{},
	},
}

// MetaScorer composes subscores according to MetaConfig.
type MetaScorer struct {
	subscores []Subscore
	config    MetaConfig
}

func NewMetaScorer(subscores []Subscore, cfg MetaConfig) *MetaScorer {
	return &MetaScorer{subscores: subscores, config: cfg}
}

// ScoreWithBreakdown computes the metascore and returns breakdown.
func (m *MetaScorer) ScoreWithBreakdown(data interface{}, profileCfg interface{}) (float64, map[string]SubscoreResult) {
	fmt.Fprintln(os.Stderr, "ScoreWithBreakdown called")
	results := make(map[string]SubscoreResult)
	weights := make(map[string]float64)
	var totalWeight float64

	for _, s := range m.subscores {
		res := s.Compute(data, profileCfg)
		res.Value = Clamp01(res.Value)
		if res.Confidence == 0 {
			res.Confidence = 1.0
		}
		res.Confidence = Clamp01(res.Confidence)
		results[s.Name()] = res
		w := m.config.BaseWeights[s.Name()]
		if m.config.UseConfidenceWeight {
			w *= res.Confidence
		}
		weights[s.Name()] = w
		totalWeight += w
	}

	var score float64
	mode := m.config.Mode

	// Logging mode and results
	fmt.Fprintln(os.Stderr, "MetaScorer mode:", mode)
	for name, res := range results {
		fmt.Fprintln(os.Stderr, "Subscore", name, "= value:", res.Value, ", confidence:", res.Confidence)
	}

	switch mode {
	case WeightedAdditive:
		var sum float64
		for name, res := range results {
			w := weights[name]
			sum += w * res.Value
		}
		if totalWeight > 0 {
			score = sum / totalWeight
		}
	case WeightedMultiplicative:
		score = 1.0
		for name, res := range results {
			w := weights[name]
			score *= math.Pow(res.Value, w)
		}
	case HybridStructural:
		// Example: structure = pow(CCS, w1) * pow(OQS, w2)
		// modifier  = 1 + w3*(VOS - 0.5)
		// score     = structure * modifier * pow(DCS, w4)
		// This is a placeholder; actual implementation depends on subscores
		structure := 1.0
		modifier := 1.0
		for name, res := range results {
			w := weights[name]
			switch name {
			case "CCS", "OQS":
				structure *= math.Pow(res.Value, w)
			case "VOS":
				modifier *= 1 + w*(res.Value-0.5)
			case "DCS":
				structure *= math.Pow(res.Value, w)
			}
		}
		score = structure * modifier
	}

	score = Clamp01(score)
	return score, results
}

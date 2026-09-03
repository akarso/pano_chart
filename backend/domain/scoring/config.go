package scoring

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// YAML mirror structs — these map 1:1 to config.yaml
// ---------------------------------------------------------------------------

// AppConfig is the top-level configuration read from config.yaml.
type AppConfig struct {
	Sideways    SidewaysYAML    `yaml:"sideways"`
	Compression CompressionYAML `yaml:"compression"`
	Breakout    BreakoutYAML    `yaml:"breakout"`
}

// --- Sideways ---

type SidewaysYAML struct {
	ExtremaWindow   int                `yaml:"extrema_window"`
	CandleCount     int                `yaml:"candle_count"`
	ExtremaMinCount int                `yaml:"extrema_min_count"`
	ATR             SidewaysATRYAML    `yaml:"atr"`
	IdealATRRange   map[string]float64 `yaml:"ideal_atr_range"`
	RangeTolerance  float64            `yaml:"range_tolerance"`
	Weights         SidewaysWeights    `yaml:"weights"`
}

type SidewaysATRYAML struct {
	Period          int     `yaml:"period"`
	SpikeMultiplier float64 `yaml:"spike_multiplier"`
}

type SidewaysWeights struct {
	ChannelStructure   float64 `yaml:"channel_structure"`
	OscillationQuality float64 `yaml:"oscillation_quality"`
	DriftControl       float64 `yaml:"drift_control"`
	VolatilityScore    float64 `yaml:"volatility_score"`
}

// --- Compression ---

type CompressionYAML struct {
	SwingLookback      int                 `yaml:"swing_lookback"`
	CandleCount        int                 `yaml:"candle_count"`
	ATR                CompressionATRYAML  `yaml:"atr"`
	MinStructuralRange float64             `yaml:"min_structural_range"`
	WidthWeight        float64             `yaml:"width_weight"`
	VolatilityWeight   float64             `yaml:"volatility_weight"`
	ConvergenceWeight  float64             `yaml:"convergence_weight"`
	TouchFactor        float64             `yaml:"touch_factor"`
	PressureThreshold  float64             `yaml:"pressure_threshold"`
	Normalization      CompressionNormYAML `yaml:"normalization"`
}

type CompressionATRYAML struct {
	Period int `yaml:"period"`
}

type CompressionNormYAML struct {
	MaxExpectedSlope   float64 `yaml:"max_expected_slope"`
	MaxExpectedATRDrop float64 `yaml:"max_expected_atr_drop"`
	SlopeNormalization float64 `yaml:"slope_normalization"`
}

// --- Breakout ---

type BreakoutYAML struct {
	CandleCount          int                     `yaml:"candle_count"`
	ATR                  BreakoutATRYAML         `yaml:"atr"`
	Penetration          BreakoutPenetrationYAML `yaml:"penetration"`
	Conviction           BreakoutConvictionYAML  `yaml:"conviction"`
	VolatilityWeight     float64                 `yaml:"volatility_weight"`
	BoundaryWeight       float64                 `yaml:"boundary_weight"`
	ConvictionWeight     float64                 `yaml:"conviction_weight"`
	VolumeWeight         float64                 `yaml:"volume_weight"`
	Volume               BreakoutVolumeYAML      `yaml:"volume"`
	CompressionAlignment BreakoutCompressionYAML `yaml:"compression_alignment"`
	FailureDetection     BreakoutFailureYAML     `yaml:"failure_detection"`
	Dominance            BreakoutDominanceYAML   `yaml:"dominance"`
}

type BreakoutATRYAML struct {
	Period        int     `yaml:"period"`
	ExpansionNorm float64 `yaml:"expansion_norm"`
}

type BreakoutPenetrationYAML struct {
	ATRNorm float64 `yaml:"atr_norm"`
}

type BreakoutConvictionYAML struct {
	MinBodyRatio float64 `yaml:"min_body_ratio"`
	BodyWeight   float64 `yaml:"body_weight"`
}

type BreakoutVolumeYAML struct {
	Lookback   int     `yaml:"lookback"`
	ZScoreNorm float64 `yaml:"zscore_norm"`
	Required   bool    `yaml:"required"`
}

type BreakoutCompressionYAML struct {
	Enabled     bool    `yaml:"enabled"`
	Threshold   float64 `yaml:"threshold"`
	BoostFactor float64 `yaml:"boost_factor"`
}

type BreakoutFailureYAML struct {
	ReentryLookback int     `yaml:"reentry_lookback"`
	Penalty         float64 `yaml:"penalty"`
}

type BreakoutDominanceYAML struct {
	SuppressSidewaysThreshold float64 `yaml:"suppress_sideways_threshold"`
	SidewaysSuppressionFactor float64 `yaml:"sideways_suppression_factor"`
}

// ---------------------------------------------------------------------------
// Singleton loader — replaces the old .env mechanism
// ---------------------------------------------------------------------------

var (
	globalConfig     *AppConfig
	globalConfigOnce sync.Once
	globalConfigErr  error
)

// LoadConfig reads config.yaml from the given path and stores it as the
// package-level configuration. Safe to call multiple times; only the first
// call triggers file I/O.
func LoadConfig(path string) (*AppConfig, error) {
	globalConfigOnce.Do(func() {
		data, err := os.ReadFile(path)
		if err != nil {
			globalConfigErr = fmt.Errorf("read config %s: %w", path, err)
			return
		}
		var cfg AppConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			globalConfigErr = fmt.Errorf("parse config %s: %w", path, err)
			return
		}
		globalConfig = &cfg
	})
	return globalConfig, globalConfigErr
}

// MustLoadConfig calls LoadConfig and panics on error.
func MustLoadConfig(path string) *AppConfig {
	cfg, err := LoadConfig(path)
	if err != nil {
		panic(err)
	}
	return cfg
}

// ConfigPath resolves where to find config.yaml.
// Priority: $CONFIG_PATH env var > executable directory > working directory.
func ConfigPath() string {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		return p
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "config.yaml"
}

// GetConfig returns the loaded AppConfig or nil if not yet loaded.
func GetConfig() *AppConfig {
	return globalConfig
}

// ResetConfig clears the singleton for testing purposes.
func ResetConfig() {
	globalConfig = nil
	globalConfigErr = nil
	globalConfigOnce = sync.Once{}
}

// ---------------------------------------------------------------------------
// Default / hardcoded fallback config — used when config.yaml is absent
// (e.g. in unit tests that construct configs explicitly).
// ---------------------------------------------------------------------------

// DefaultAppConfig returns a fully populated AppConfig with hardcoded
// defaults that mirror the canonical config.yaml values.
func DefaultAppConfig() *AppConfig {
	return &AppConfig{
		Sideways: SidewaysYAML{
			ExtremaWindow:   3,
			CandleCount:     110,
			ExtremaMinCount: 8,
			ATR: SidewaysATRYAML{
				Period:          14,
				SpikeMultiplier: 3.0,
			},
			IdealATRRange: map[string]float64{
				"1m": 2.5, "5m": 3.0, "15m": 3.5,
				"1h": 4.0, "4h": 4.5, "1d": 5.0,
			},
			RangeTolerance: 1.5,
			Weights: SidewaysWeights{
				ChannelStructure:   1.0,
				OscillationQuality: 2.0,
				DriftControl:       1.0,
				VolatilityScore:    1.0,
			},
		},
		Compression: CompressionYAML{
			SwingLookback:      3,
			CandleCount:        100,
			ATR:                CompressionATRYAML{Period: 14},
			MinStructuralRange: 0.005,
			WidthWeight:        1.0,
			VolatilityWeight:   1.0,
			ConvergenceWeight:  1.0,
			TouchFactor:        0.5,
			PressureThreshold:  0.2,
			Normalization: CompressionNormYAML{
				MaxExpectedSlope:   0.01,
				MaxExpectedATRDrop: 0.005,
				SlopeNormalization: 0.1,
			},
		},
		Breakout: BreakoutYAML{
			CandleCount: 60,
			ATR: BreakoutATRYAML{
				Period:        14,
				ExpansionNorm: 0.01,
			},
			Penetration: BreakoutPenetrationYAML{ATRNorm: 1.2},
			Conviction: BreakoutConvictionYAML{
				MinBodyRatio: 0.6,
				BodyWeight:   1.0,
			},
			VolatilityWeight: 1.0,
			BoundaryWeight:   1.5,
			ConvictionWeight: 1.2,
			VolumeWeight:     1.0,
			Volume: BreakoutVolumeYAML{
				Lookback:   30,
				ZScoreNorm: 2.0,
				Required:   false,
			},
			CompressionAlignment: BreakoutCompressionYAML{
				Enabled:     true,
				Threshold:   0.6,
				BoostFactor: 1.5,
			},
			FailureDetection: BreakoutFailureYAML{
				ReentryLookback: 3,
				Penalty:         0.4,
			},
			Dominance: BreakoutDominanceYAML{
				SuppressSidewaysThreshold: 0.7,
				SidewaysSuppressionFactor: 0.3,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Builder functions — convert YAML sections to domain config structs
// ---------------------------------------------------------------------------

// appCfgOrDefault returns the loaded global config, or DefaultAppConfig()
// if config.yaml hasn't been loaded (e.g. in tests).
func appCfgOrDefault() *AppConfig {
	if globalConfig != nil {
		return globalConfig
	}
	return DefaultAppConfig()
}

// NewSidewaysV5ConfigForTimeframe builds a SidewaysV5Config from the loaded
// config.yaml, resolving IdealATRRange for the given timeframe string.
// If the timeframe isn't in the map, falls back to "1h".
func NewSidewaysV5ConfigForTimeframe(tf string) SidewaysV5Config {
	app := appCfgOrDefault()
	s := app.Sideways

	ideal, ok := s.IdealATRRange[tf]
	if !ok {
		ideal = s.IdealATRRange["1h"] // safe fallback
		if ideal == 0 {
			ideal = 4.0
		}
	}

	return SidewaysV5Config{
		N:                s.ExtremaWindow,
		CandleCount:      s.CandleCount,
		IdealATRRange:    ideal,
		IdealATRRangeMap: s.IdealATRRange,
		RangeTolerance:   s.RangeTolerance,
		ATRMultiplier:    s.ATR.SpikeMultiplier,
		W1:               s.Weights.ChannelStructure,
		W2:               s.Weights.OscillationQuality,
		W3:               s.Weights.DriftControl,
		W4:               s.Weights.VolatilityScore,
		ExtremaCount:     s.ExtremaMinCount,
	}
}

// NewCompressionConfig builds a CompressionConfig from the loaded config.yaml.
func NewCompressionConfig() CompressionConfig {
	app := appCfgOrDefault()
	c := app.Compression

	return CompressionConfig{
		SwingLookback:      c.SwingLookback,
		ATRPeriod:          c.ATR.Period,
		MinStructuralRange: c.MinStructuralRange,
		WidthWeight:        c.WidthWeight,
		VolWeight:          c.VolatilityWeight,
		ConvergenceWeight:  c.ConvergenceWeight,
		TouchFactor:        c.TouchFactor,
		PressureThreshold:  c.PressureThreshold,
		SmallRangePenalty:  0.3, // not in YAML — kept as hardcoded structural default
		MaxExpectedSlope:   c.Normalization.MaxExpectedSlope,
		MaxExpectedATRDrop: c.Normalization.MaxExpectedATRDrop,
		SlopeNormalization: c.Normalization.SlopeNormalization,
		CandleCount:        c.CandleCount,
	}
}

// NewBreakoutConfig builds a BreakoutConfig from the loaded config.yaml.
func NewBreakoutConfig() BreakoutConfig {
	app := appCfgOrDefault()
	b := app.Breakout

	return BreakoutConfig{
		ATRPeriod:              b.ATR.Period,
		VolumeLookback:         b.Volume.Lookback,
		PenetrationNorm:        b.Penetration.ATRNorm,
		ATRNorm:                b.ATR.ExpansionNorm,
		VolumeNorm:             b.Volume.ZScoreNorm,
		ReentryLookback:        b.FailureDetection.ReentryLookback,
		FailurePenalty:         b.FailureDetection.Penalty,
		CompressionThreshold:   b.CompressionAlignment.Threshold,
		CompressionBoostFactor: b.CompressionAlignment.BoostFactor,
		MinBodyRatio:           b.Conviction.MinBodyRatio,
		SwingLookback:          3, // structural default — not in YAML
		CandleCount:            b.CandleCount,
		W1:                     b.BoundaryWeight,
		W2:                     b.ConvictionWeight,
		W3:                     b.VolatilityWeight,
	}
}

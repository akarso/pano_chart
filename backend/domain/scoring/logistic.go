package scoring

import (
	"fmt"
	"math"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"pano_chart/backend/domain"
)

// Regime class keys used by one-vs-rest Models (PR-109).
const (
	ClassTrend       = "trend"
	ClassSideways    = "sideways"
	ClassCompression = "compression"
	ClassExpansion   = "expansion"
)

// RequiredRegimeClasses is the full one-vs-rest set for inference.
var RequiredRegimeClasses = []string{
	ClassTrend, ClassSideways, ClassCompression, ClassExpansion,
}

// DefaultRegimeFeatures is the canonical feature order for export / training.
var DefaultRegimeFeatures = []string{
	"trend", "sideways", "compression", "expansion",
	"er", "vr", "atr_pct", "rs", "alignment",
}

// ClassParams is one logistic head in a one-vs-rest regime model.
type ClassParams struct {
	Weights map[string]float64 `yaml:"weights"`
	Bias    float64            `yaml:"bias"`
}

// Model is a logistic regression shipped as YAML (PR-109).
//
// Structure inference uses one-vs-rest Classes (not the binary Weights/Bias
// head). Binary Weights/Bias are for notebooks predicting setup Success only.
// Placeholder models refuse ClassifyStructure so heuristic classification
// remains active until a real scorecard model is shipped.
type Model struct {
	Type        string                 `yaml:"model"` // "logistic"
	Features    []string               `yaml:"features"`
	Weights     map[string]float64     `yaml:"weights"`
	Bias        float64                `yaml:"bias"`
	Classes     map[string]ClassParams `yaml:"classes"`
	Placeholder bool                   `yaml:"placeholder"` // true → ClassifyStructure ok=false
}

// Predict returns σ(bias + Σ w_i x_i) using the binary Weights/Bias head.
// Missing feature values are treated as 0. Non-finite inputs are skipped.
func Predict(features map[string]float64, model Model) float64 {
	return sigmoid(logit(features, model.Weights, model.Bias))
}

// ClassifyStructure runs one-vs-rest Predict per class, L1-normalizes the four
// probabilities into Structure weights, and returns the argmax class.
// ok is false when the model is a placeholder, lacks all four classes, or
// produces a non-finite / zero mass sum (caller must fall back to heuristic).
func ClassifyStructure(features map[string]float64, model Model) (trend, sideways, compression, expansion float64, dominant string, ok bool) {
	if model.Placeholder {
		return 0, 0, 0, 0, "", false
	}
	if err := model.ValidateForInference(); err != nil {
		return 0, 0, 0, 0, "", false
	}
	order := RequiredRegimeClasses
	raw := make([]float64, len(order))
	var sum float64
	for i, name := range order {
		cp := model.Classes[name]
		p := sigmoid(logit(features, cp.Weights, cp.Bias))
		raw[i] = p
		sum += p
	}
	if sum <= 0 || math.IsNaN(sum) || math.IsInf(sum, 0) {
		return 0, 0, 0, 0, "", false
	}
	norm := make([]float64, len(order))
	bestI := 0
	for i := range order {
		norm[i] = raw[i] / sum
		if norm[i] > norm[bestI] {
			bestI = i
		}
	}
	return norm[0], norm[1], norm[2], norm[3], order[bestI], true
}

// LoadRegimeModel reads a regime_model.yaml file.
func LoadRegimeModel(path string) (*Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read regime model %s: %w", path, err)
	}
	var m Model
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse regime model %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("invalid regime model %s: %w", path, err)
	}
	return &m, nil
}

// Validate checks Type, finite coefficients, and that Weights and/or Classes exist.
func (m Model) Validate() error {
	kind := strings.ToLower(strings.TrimSpace(m.Type))
	if kind != "" && kind != "logistic" {
		return fmt.Errorf("unsupported model type %q (want logistic)", m.Type)
	}
	if len(m.Weights) == 0 && len(m.Classes) == 0 {
		return fmt.Errorf("model needs weights and/or classes")
	}
	if err := validateWeights(m.Weights); err != nil {
		return fmt.Errorf("binary weights: %w", err)
	}
	if math.IsNaN(m.Bias) || math.IsInf(m.Bias, 0) {
		return fmt.Errorf("binary bias is non-finite")
	}
	for name, cp := range m.Classes {
		if err := validateWeights(cp.Weights); err != nil {
			return fmt.Errorf("class %s weights: %w", name, err)
		}
		if math.IsNaN(cp.Bias) || math.IsInf(cp.Bias, 0) {
			return fmt.Errorf("class %s bias is non-finite", name)
		}
	}
	return nil
}

// ValidateForInference requires a non-placeholder model with all four classes.
func (m Model) ValidateForInference() error {
	if m.Placeholder {
		return fmt.Errorf("placeholder model cannot be used for inference")
	}
	if err := m.Validate(); err != nil {
		return err
	}
	for _, name := range RequiredRegimeClasses {
		if _, ok := m.Classes[name]; !ok {
			return fmt.Errorf("missing class %q", name)
		}
	}
	return nil
}

func validateWeights(w map[string]float64) error {
	for k, v := range w {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%q is non-finite", k)
		}
	}
	return nil
}

// EfficiencyRatio is Kaufman's ER on closes: |net| / path of consecutive
// absolute moves. Returns 0 when undefined (short series or zero path).
func EfficiencyRatio(closes []float64) float64 {
	n := len(closes)
	if n < 2 {
		return 0
	}
	var path float64
	for i := 1; i < n; i++ {
		if !finitePositive(closes[i]) || !finitePositive(closes[i-1]) {
			return 0
		}
		path += math.Abs(closes[i] - closes[i-1])
	}
	if path <= 0 || math.IsNaN(path) || math.IsInf(path, 0) {
		return 0
	}
	net := math.Abs(closes[n-1] - closes[0])
	er := net / path
	if math.IsNaN(er) || math.IsInf(er, 0) {
		return 0
	}
	return clamp01(er)
}

// ClosesFromCandles extracts positive finite closes, skipping bad bars
// (does not truncate the remainder of the series).
func ClosesFromCandles(candles []domain.Candle) []float64 {
	out := make([]float64, 0, len(candles))
	for _, c := range candles {
		cl := c.Close()
		if !finitePositive(cl) {
			continue
		}
		out = append(out, cl)
	}
	return out
}

// BuildRegimeFeatures assembles the feature map used by Predict / ClassifyStructure.
// rs and alignment are optional (0 when unknown — typical on the tape path).
// atr must be the same Wilder TrueATR(14) used by ScoreMarketTape when the
// training target is tape structure; setup path uses the same TrueATR for
// emit/classify parity.
func BuildRegimeFeatures(
	trend, sideways, compression, expansion float64,
	closes []float64,
	atr, price float64,
	rs, alignment float64,
) map[string]float64 {
	atrPct := 0.0
	if price > 0 && atr > 0 && !math.IsNaN(atr) && !math.IsInf(atr, 0) {
		atrPct = atr / price
	}
	vr := VarianceRatio(closes, 4)
	return map[string]float64{
		"trend":       trend,
		"sideways":    sideways,
		"compression": compression,
		"expansion":   expansion,
		"er":          EfficiencyRatio(closes),
		"vr":          vr,
		"atr_pct":     atrPct,
		"rs":          rs,
		"alignment":   alignment,
	}
}

func logit(features map[string]float64, weights map[string]float64, bias float64) float64 {
	z := bias
	for k, w := range weights {
		if math.IsNaN(w) || math.IsInf(w, 0) {
			continue
		}
		x := features[k] // missing → 0
		if math.IsNaN(x) || math.IsInf(x, 0) {
			continue
		}
		z += w * x
	}
	return z
}

func sigmoid(z float64) float64 {
	if math.IsNaN(z) {
		return 0.5
	}
	if z > 35 {
		return 1
	}
	if z < -35 {
		return 0
	}
	return 1 / (1 + math.Exp(-z))
}

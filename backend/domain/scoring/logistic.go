package scoring

import (
	"fmt"
	"math"
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

// DefaultRegimeFeatures is the canonical feature order for setup export /
// training. RS and alignment are omitted: the setup emit path does not
// populate them, so advertising them would ship constant-zero columns.
var DefaultRegimeFeatures = []string{
	"trend", "sideways", "compression", "expansion",
	"er", "vr", "atr_pct",
}

// knownFeatureSet is the union of names allowed in weight tables.
func knownFeatureSet() map[string]struct{} {
	out := make(map[string]struct{}, len(DefaultRegimeFeatures)+2)
	for _, k := range DefaultRegimeFeatures {
		out[k] = struct{}{}
	}
	// Optional enrichment keys (tape / future emitters); allowed in weights
	// if present, but not required in setup CSV.
	out["rs"] = struct{}{}
	out["alignment"] = struct{}{}
	return out
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

// ParseRegimeModelYAML unmarshals YAML bytes into a Model and runs Validate.
// File I/O belongs in infrastructure (see infrastructure/scoring).
func ParseRegimeModelYAML(data []byte) (*Model, error) {
	var m Model
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse regime model: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("invalid regime model: %w", err)
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
	known := knownFeatureSet()
	if err := validateWeightKeys(m.Weights, known, "binary"); err != nil {
		return err
	}
	if math.IsNaN(m.Bias) || math.IsInf(m.Bias, 0) {
		return fmt.Errorf("binary bias is non-finite")
	}
	for name, cp := range m.Classes {
		if err := validateWeightKeys(cp.Weights, known, "class "+name); err != nil {
			return err
		}
		if math.IsNaN(cp.Bias) || math.IsInf(cp.Bias, 0) {
			return fmt.Errorf("class %s bias is non-finite", name)
		}
	}
	return nil
}

// ValidateForInference requires a non-placeholder model with all four classes,
// each having at least one weight whose feature name is known.
func (m Model) ValidateForInference() error {
	if m.Placeholder {
		return fmt.Errorf("placeholder model cannot be used for inference")
	}
	if err := m.Validate(); err != nil {
		return err
	}
	known := knownFeatureSet()
	// If Features is declared, it must be non-empty and every entry known;
	// class weight keys must then be ⊆ Features.
	allowed := known
	if len(m.Features) > 0 {
		allowed = make(map[string]struct{}, len(m.Features))
		for _, f := range m.Features {
			if _, ok := known[f]; !ok {
				return fmt.Errorf("features list contains unknown name %q", f)
			}
			allowed[f] = struct{}{}
		}
	}
	for _, name := range RequiredRegimeClasses {
		cp, ok := m.Classes[name]
		if !ok {
			return fmt.Errorf("missing class %q", name)
		}
		if len(cp.Weights) == 0 {
			return fmt.Errorf("class %q has no weights (constant head)", name)
		}
		if err := validateWeightKeys(cp.Weights, allowed, "class "+name); err != nil {
			return err
		}
	}
	return nil
}

func validateWeightKeys(w map[string]float64, allowed map[string]struct{}, label string) error {
	for k, v := range w {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%s weight %q is non-finite", label, k)
		}
		if _, ok := allowed[k]; !ok {
			return fmt.Errorf("%s weight names unknown feature %q", label, k)
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

// BuildRegimeFeatures assembles the feature map used by Predict / ClassifyStructure
// and setup export (DefaultRegimeFeatures). atr should be Wilder TrueATR(14)
// when available; atr_pct is 0 when atr/price is undefined.
func BuildRegimeFeatures(
	trend, sideways, compression, expansion float64,
	closes []float64,
	atr, price float64,
) map[string]float64 {
	atrPct := 0.0
	if price > 0 && atr > 0 && !math.IsNaN(atr) && !math.IsInf(atr, 0) {
		atrPct = atr / price
	}
	return map[string]float64{
		"trend":       trend,
		"sideways":    sideways,
		"compression": compression,
		"expansion":   expansion,
		"er":          EfficiencyRatio(closes),
		"vr":          VarianceRatio(closes, 4),
		"atr_pct":     atrPct,
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

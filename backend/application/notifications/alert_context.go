package notifications

import (
	"encoding/json"
	"math"
)

// AlertSparklinePoints is the sparkline length attached to alert context
// (ROADMAP PR-102: downsample the 110-bar snapshot to 30 closes).
const AlertSparklinePoints = 30

// AlertContext is the structured payload nested under Data["context"].
// Pointer / omitempty fields stay absent when unknown so clients can tell
// "missing" from a real zero (same presence model as rankings RS).
type AlertContext struct {
	TapeRegime     string    `json:"tapeRegime,omitempty"`
	TapeBias       string    `json:"tapeBias,omitempty"`
	TapeConfidence *float64  `json:"tapeConfidence,omitempty"`
	SymbolScore    *float64  `json:"symbolScore,omitempty"`
	RS             *float64  `json:"rs,omitempty"`
	Alignment      *float64  `json:"alignment,omitempty"`
	Sparkline      []float64 `json:"sparkline,omitempty"`
}

// empty reports whether no field would appear in JSON.
func (c AlertContext) empty() bool {
	return c.TapeRegime == "" &&
		c.TapeBias == "" &&
		c.TapeConfidence == nil &&
		c.SymbolScore == nil &&
		c.RS == nil &&
		c.Alignment == nil &&
		len(c.Sparkline) == 0
}

// DownsampleSparkline returns up to target evenly spaced closes, always
// keeping the first and last when target >= 2. Inputs that are already
// short enough are copied. Nil/empty or target < 1 yield nil/empty.
// Non-finite values are dropped from the source before sampling.
func DownsampleSparkline(closes []float64, target int) []float64 {
	clean := sanitizeSparkline(closes)
	n := len(clean)
	if n == 0 || target < 1 {
		return nil
	}
	if target == 1 {
		return []float64{clean[0]}
	}
	if n <= target {
		out := make([]float64, n)
		copy(out, clean)
		return out
	}
	out := make([]float64, target)
	denom := target - 1
	last := n - 1
	for i := 0; i < target; i++ {
		idx := i * last / denom
		out[i] = clean[idx]
	}
	return out
}

// sanitizeSparkline keeps finite closes and rounds to 4 decimal places so
// one NaN/Inf cannot break json.Marshal for the whole context blob.
func sanitizeSparkline(closes []float64) []float64 {
	if len(closes) == 0 {
		return nil
	}
	out := make([]float64, 0, len(closes))
	for _, v := range closes {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		out = append(out, RoundConfidence(v))
	}
	return out
}

// AttachContext copies data and sets Data["context"] to compact JSON when
// ctx has at least one field. Marshal failures leave data unchanged.
func AttachContext(data map[string]string, ctx AlertContext) map[string]string {
	out := make(map[string]string, len(data)+1)
	for k, v := range data {
		out[k] = v
	}
	if ctx.empty() {
		return out
	}
	// Re-sanitize in case a caller set Sparkline without going through
	// DownsampleSparkline — tape fields must survive a bad close.
	ctx.Sparkline = sanitizeSparkline(ctx.Sparkline)
	raw, err := json.Marshal(ctx)
	if err != nil {
		// Last resort: drop sparkline and retry so tape still ships.
		ctx.Sparkline = nil
		if ctx.empty() {
			return out
		}
		raw, err = json.Marshal(ctx)
		if err != nil {
			return out
		}
	}
	out["context"] = string(raw)
	return out
}

// RoundConfidence trims float noise for compact FCM payloads.
func RoundConfidence(v float64) float64 {
	return math.Round(v*1e4) / 1e4
}

// floatPtr rounds v for the payload, or returns nil when v is non-finite
// so a NaN confidence/score cannot poison json.Marshal for the whole blob.
func floatPtr(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	x := RoundConfidence(v)
	return &x
}

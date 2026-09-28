package metrics

import (
	"fmt"
	"regexp"
	"strings"

	"pano_chart/backend/domain/scoring"
)

// SymbolFilter skips symbols before the composite candle fan-out (PR-095).
type SymbolFilter struct {
	exact map[string]struct{}
	re    *regexp.Regexp
}

// NewSymbolFilter builds a filter from an exact-name list and an optional
// Go regexp. Empty pattern disables the regex guard. Empty list + empty
// pattern means no exclusions. Pattern is matched case-insensitively.
func NewSymbolFilter(exclude []string, pattern string) (*SymbolFilter, error) {
	exact := make(map[string]struct{}, len(exclude))
	for _, s := range exclude {
		s = strings.TrimSpace(strings.ToUpper(s))
		if s != "" {
			exact[s] = struct{}{}
		}
	}
	var re *regexp.Regexp
	if pattern != "" {
		compiled, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil, fmt.Errorf("composite exclude_pattern: %w", err)
		}
		re = compiled
	}
	return &SymbolFilter{exact: exact, re: re}, nil
}

// DefaultSymbolFilter uses only scoring.DefaultAppConfig().Composite — never
// the process LoadConfig singleton (avoids order-dependent test/prod behavior).
func DefaultSymbolFilter() *SymbolFilter {
	cfg := scoring.DefaultAppConfig().Composite
	f, err := NewSymbolFilter(cfg.Exclude, cfg.ExcludePattern)
	if err != nil {
		f, _ = NewSymbolFilter(cfg.Exclude, "")
	}
	return f
}

// Skip reports whether sym must not be fetched for the composite.
func (f *SymbolFilter) Skip(sym string) bool {
	if f == nil {
		return false
	}
	u := strings.ToUpper(sym)
	if _, ok := f.exact[u]; ok {
		return true
	}
	return f.re != nil && f.re.MatchString(u)
}

// SymbolFilterFromConfig builds a filter from CompositeYAML fields as given.
// Empty exclude + empty pattern → no exclusions. Callers that want defaults
// for a missing YAML block must pass scoring.EffectiveComposite first.
func SymbolFilterFromConfig(exclude []string, pattern string) (*SymbolFilter, error) {
	return NewSymbolFilter(exclude, pattern)
}

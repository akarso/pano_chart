package metrics_test

import (
	"testing"

	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/domain/scoring"
)

func TestSymbolFilter_ListAndPrefix(t *testing.T) {
	f, err := metrics.NewSymbolFilter(
		[]string{"USDCUSDT", "wbTCUSDT"},
		`^(USD|EUR)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Skip("USDCUSDT") {
		t.Fatal("exact list miss")
	}
	if !f.Skip("WBTCUSDT") {
		t.Fatal("case-normalized list miss")
	}
	if !f.Skip("EURUSDT") {
		t.Fatal("prefix miss")
	}
	if !f.Skip("usdabcUSDT") {
		t.Fatal("case-insensitive prefix miss")
	}
	if f.Skip("BTCUSDT") {
		t.Fatal("BTCUSDT must not be skipped")
	}
}

func TestSymbolFilter_EmptyMeansNoExclusions(t *testing.T) {
	f, err := metrics.NewSymbolFilter(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if f.Skip("USDCUSDT") {
		t.Fatal("empty filter must not skip")
	}
}

func TestSymbolFilter_BadPattern(t *testing.T) {
	_, err := metrics.NewSymbolFilter(nil, `(`)
	if err == nil {
		t.Fatal("expected compile error")
	}
}

func TestSymbolFilter_FromConfigDoesNotInjectDefaults(t *testing.T) {
	f, err := metrics.SymbolFilterFromConfig([]string{"FOOUSDT"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !f.Skip("FOOUSDT") {
		t.Fatal("list-only should skip FOO")
	}
	if f.Skip("USDCUSDT") {
		t.Fatal("must not inject default USDC when pattern empty")
	}
}

func TestDefaultSymbolFilter_MatchesDefaultAppConfig(t *testing.T) {
	cfg := scoring.DefaultAppConfig().Composite
	f := metrics.DefaultSymbolFilter()
	for _, s := range cfg.Exclude {
		if !f.Skip(s) {
			t.Fatalf("default filter must skip %s", s)
		}
	}
	if f.Skip("BTCUSDT") {
		t.Fatal("BTCUSDT must not be skipped")
	}
}

func TestEffectiveComposite_EmptyUsesDefaults(t *testing.T) {
	got := scoring.EffectiveComposite(scoring.CompositeYAML{})
	want := scoring.DefaultAppConfig().Composite
	if len(got.Exclude) == 0 || got.ExcludePattern == "" {
		t.Fatalf("empty YAML must expand to defaults, got %+v", got)
	}
	if len(got.Exclude) != len(want.Exclude) || got.ExcludePattern != want.ExcludePattern {
		t.Fatalf("got %+v want %+v", got, want)
	}
	f, err := metrics.SymbolFilterFromConfig(got.Exclude, got.ExcludePattern)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Skip("USDCUSDT") {
		t.Fatal("entrypoint path must still exclude USDC after empty YAML")
	}
}

func TestEffectiveComposite_DisableExclusions(t *testing.T) {
	got := scoring.EffectiveComposite(scoring.CompositeYAML{DisableExclusions: true})
	if len(got.Exclude) != 0 || got.ExcludePattern != "" {
		t.Fatalf("disable_exclusions must clear lists, got %+v", got)
	}
	f, err := metrics.SymbolFilterFromConfig(got.Exclude, got.ExcludePattern)
	if err != nil {
		t.Fatal(err)
	}
	if f.Skip("USDCUSDT") {
		t.Fatal("disable_exclusions must exclude nothing")
	}
}

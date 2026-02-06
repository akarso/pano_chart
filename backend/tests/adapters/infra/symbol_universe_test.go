package infra_test

import (
	"testing"
	infra "pano_chart/backend/adapters/infra"
)

func TestStaticSymbolUniverse_DeterministicOrder(t *testing.T) {
	syms := []string{"ETHUSDT", "BTCUSDT", "BTCUSDT", "ADAUSDT"}
	su, err := infra.NewStaticSymbolUniverse(syms)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list, err := su.ListSymbols()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 unique symbols, got %d", len(list))
	}
	if list[0].String() != "ADAUSDT" || list[1].String() != "BTCUSDT" || list[2].String() != "ETHUSDT" {
		t.Errorf("unexpected order: %+v", list)
	}
}

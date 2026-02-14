package symbol_universe

import (
	"context"
	"testing"
)

func TestStaticBinanceTop15Universe_DeterministicOrderAndSize(t *testing.T) {
	provider := NewStaticBinanceTop15Universe()
	syms, err := provider.Symbols(context.Background(), "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(syms) != 15 {
		t.Fatalf("expected 15 symbols, got %d", len(syms))
	}
	for i := 1; i < len(syms); i++ {
		if syms[i-1].String() > syms[i].String() {
			t.Errorf("symbols not sorted: %s > %s", syms[i-1].String(), syms[i].String())
		}
	}
}

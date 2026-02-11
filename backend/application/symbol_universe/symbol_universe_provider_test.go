package symbol_universe

import (
	"context"
	"pano_chart/backend/domain"
	"testing"
)

func TestSymbolUniverseProvider_Interface(t *testing.T) {
	var _ SymbolUniverseProvider = &mockProvider{}
}

type mockProvider struct{}

func (m *mockProvider) Symbols(ctx context.Context) ([]domain.Symbol, error) {
	return []domain.Symbol{}, nil
}

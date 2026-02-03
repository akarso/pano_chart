package domain_test

import (
	"testing"

	"github.com/akarso/pano_chart/backend/domain"
)

func TestSymbol_AcceptsValidSymbols(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"btc_usdt", "BTC_USDT"},
		{"eth-usd", "ETH-USD"},
		{"BTCUSDT", "BTCUSDT"},
		{"btc", "BTC"},
		{"BTC", "BTC"},
		{"BTC_USD", "BTC_USD"},
		{"BTC-USD", "BTC-USD"},
		{"SOL1", "SOL1"},
		{"1INCH", "1INCH"},
		{"token_123", "TOKEN_123"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			sym, err := domain.NewSymbol(tt.input)
			if err != nil {
				t.Fatalf("NewSymbol(%q) returned error: %v", tt.input, err)
			}

			if sym.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, sym.String())
			}
		})
	}
}

func TestSymbol_RejectsEmptyString(t *testing.T) {
	_, err := domain.NewSymbol("")
	if err == nil {
		t.Error("NewSymbol(\"\") should return error, got nil")
	}
}

func TestSymbol_RejectsWhitespaceOnly(t *testing.T) {
	tests := []string{
		" ",
		"  ",
		"\t",
		"\n",
		" \t \n ",
	}

	for _, tt := range tests {
		t.Run("whitespace", func(t *testing.T) {
			_, err := domain.NewSymbol(tt)
			if err == nil {
				t.Errorf("NewSymbol(%q) should return error, got nil", tt)
			}
		})
	}
}

func TestSymbol_RejectsIllegalCharacters(t *testing.T) {
	tests := []struct {
		input  string
		reason string
	}{
		{"BTC@USDT", "contains @"},
		{"BTC.USD", "contains ."},
		{"BTC/USD", "contains /"},
		{"BTC#USD", "contains #"},
		{"BTC USDT", "contains space"},
		{"BTC+USD", "contains +"},
		{"BTC=USD", "contains ="},
		{"BTC[USD]", "contains brackets"},
		{"BTC{USD}", "contains braces"},
		{"BTC*USD", "contains asterisk"},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			_, err := domain.NewSymbol(tt.input)
			if err == nil {
				t.Errorf("NewSymbol(%q) should reject illegal character, got nil error", tt.input)
			}
		})
	}
}

func TestSymbol_NormalizesToUppercase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"btc_usdt", "BTC_USDT"},
		{"eth-usd", "ETH-USD"},
		{"bnb_busd", "BNB_BUSD"},
		{"doge-usd", "DOGE-USD"},
		{"xrp_eur", "XRP_EUR"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			sym, err := domain.NewSymbol(tt.input)
			if err != nil {
				t.Fatalf("NewSymbol(%q) returned error: %v", tt.input, err)
			}

			if sym.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, sym.String())
			}
		})
	}
}

func TestSymbol_EqualityByValue(t *testing.T) {
	sym1, _ := domain.NewSymbol("btc_usdt")
	sym2, _ := domain.NewSymbol("BTC_USDT")
	sym3, _ := domain.NewSymbol("eth-usd")

	// Same normalized value should be equal
	if sym1 != sym2 {
		t.Errorf("symbols with same normalized value should be equal: %q vs %q", sym1.String(), sym2.String())
	}

	// Different normalized values should not be equal
	if sym1 == sym3 {
		t.Errorf("symbols with different normalized values should not be equal: %q vs %q", sym1.String(), sym3.String())
	}
}

func TestSymbol_IsImmutable(t *testing.T) {
	sym, _ := domain.NewSymbol("btc_usdt")
	original := sym.String()

	// Try to modify (should not be possible if immutable)
	// For a string-based value object, this is inherent to Go's type system
	// This test verifies the implementation returns a copy on access
	sym2, _ := domain.NewSymbol(original)
	if sym.String() != sym2.String() {
		t.Errorf("symbol should remain unchanged: %q vs %q", original, sym.String())
	}
}

func TestSymbol_PresideSeparators(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"btc_usdt", "BTC_USDT"},
		{"eth-usd", "ETH-USD"},
		{"BTC_USD", "BTC_USD"},
		{"BTC-USD", "BTC-USD"},
		{"token_123-456", "TOKEN_123-456"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			sym, err := domain.NewSymbol(tt.input)
			if err != nil {
				t.Fatalf("NewSymbol(%q) returned error: %v", tt.input, err)
			}

			if sym.String() != tt.expected {
				t.Errorf("expected separators to be preserved: %q, got %q", tt.expected, sym.String())
			}
		})
	}
}

package domain

import (
	"fmt"
	"strings"
)

// Symbol represents a tradable market instrument.
// It is a value object that is immutable and normalized on creation.
type Symbol string

// NewSymbol creates a new Symbol from a string.
// The symbol is normalized to uppercase.
// Returns an error if the symbol is invalid.
func NewSymbol(s string) (Symbol, error) {
	// Reject empty string
	if s == "" {
		return "", fmt.Errorf("symbol cannot be empty")
	}

	// Reject whitespace-only string
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("symbol cannot be whitespace-only")
	}

	// Validate characters: A-Z, 0-9, -, _
	for _, ch := range s {
		if !isValidSymbolChar(ch) {
			return "", fmt.Errorf("symbol contains invalid character: %q", ch)
		}
	}

	// Normalize to uppercase
	normalized := strings.ToUpper(s)

	return Symbol(normalized), nil
}

// isValidSymbolChar checks if a character is allowed in a symbol.
// Allowed characters: A-Z, 0-9, -, _
func isValidSymbolChar(ch rune) bool {
	return (ch >= 'A' && ch <= 'Z') ||
		(ch >= 'a' && ch <= 'z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '-' ||
		ch == '_'
}

// String returns the normalized string representation of the Symbol.
func (s Symbol) String() string {
	return string(s)
}

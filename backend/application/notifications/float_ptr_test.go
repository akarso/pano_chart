package notifications

import (
	"math"
	"testing"
)

func TestFloatPtr_RejectsNonFinite(t *testing.T) {
	if floatPtr(math.NaN()) != nil {
		t.Fatal("NaN must yield nil")
	}
	if floatPtr(math.Inf(1)) != nil {
		t.Fatal("+Inf must yield nil")
	}
	if floatPtr(math.Inf(-1)) != nil {
		t.Fatal("-Inf must yield nil")
	}
	got := floatPtr(0.62)
	if got == nil || *got != 0.62 {
		t.Fatalf("finite = %v", got)
	}
	got0 := floatPtr(0)
	if got0 == nil || *got0 != 0 {
		t.Fatalf("zero must remain present, got %v", got0)
	}
}

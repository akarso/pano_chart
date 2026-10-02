package usecases

import "testing"

func TestParseCompressionAlgo(t *testing.T) {
	cases := []struct {
		in   string
		want CompressionAlgoMode
		ok   bool
	}{
		{"", CompressionAlgoAbsolute, true},
		{"absolute", CompressionAlgoAbsolute, true},
		{"Absolute", CompressionAlgoAbsolute, true},
		{"percentile", CompressionAlgoPercentile, true},
		{"Percentile", CompressionAlgoPercentile, true},
		{" PERCENTILE ", CompressionAlgoPercentile, true},
		{"bogus", CompressionAlgoAbsolute, false},
	}
	for _, tc := range cases {
		got, ok := ParseCompressionAlgo(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("ParseCompressionAlgo(%q)=(%q,%v) want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCompressionCalcFor_Names(t *testing.T) {
	if n := CompressionCalcFor(CompressionAlgoAbsolute).Name(); n != "Compression" {
		t.Fatalf("absolute Name=%q", n)
	}
	if n := CompressionCalcFor(CompressionAlgoPercentile).Name(); n != "Compression Pct" {
		t.Fatalf("percentile Name=%q", n)
	}
}

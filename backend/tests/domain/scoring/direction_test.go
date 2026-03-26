package scoring_test

import (
	"testing"

	"pano_chart/backend/domain/scoring"
)

func TestDirectionAgreement(t *testing.T) {
	tests := []struct {
		name     string
		up, down int
		want     float64
	}{
		{"all up", 10, 0, 1.0},
		{"all down", 0, 8, 1.0},
		{"perfect split", 5, 5, 0.0},
		{"70/30", 7, 3, 0.4},
		{"30/70", 3, 7, 0.4}, // symmetric
		{"no observations", 0, 0, 0.0},
		{"one up", 1, 0, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoring.DirectionAgreement(tt.up, tt.down)
			if diff := got - tt.want; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("DirectionAgreement(%d,%d) = %f, want %f", tt.up, tt.down, got, tt.want)
			}
		})
	}
}

func TestSeriesDirectionAgreement(t *testing.T) {
	tests := []struct {
		name     string
		closes   []float64
		segments int
		want     float64
	}{
		{
			name:     "steady uptrend 4 segments",
			closes:   []float64{1, 2, 3, 4, 5, 6, 7, 8},
			segments: 4,
			want:     1.0, // all 4 quarters go up
		},
		{
			name:     "steady downtrend 4 segments",
			closes:   []float64{8, 7, 6, 5, 4, 3, 2, 1},
			segments: 4,
			want:     1.0, // all 4 quarters go down
		},
		{
			name:     "V shape 2 segments",
			closes:   []float64{10, 8, 6, 4, 6, 8, 10, 12},
			segments: 2,
			want:     0.0, // first half down, second half up
		},
		{
			name:     "too short for segments",
			closes:   []float64{1, 2, 3},
			segments: 4,
			want:     1.0, // n < segments*2, no penalty
		},
		{
			name:     "W shape 4 segments",
			closes:   []float64{10, 8, 6, 8, 10, 8, 6, 8},
			segments: 4,
			want:     0.0, // 2 down, 2 up = perfect split
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoring.SeriesDirectionAgreement(tt.closes, tt.segments)
			if diff := got - tt.want; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("SeriesDirectionAgreement = %f, want %f", got, tt.want)
			}
		})
	}
}

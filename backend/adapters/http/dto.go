package http

import (
	"pano_chart/backend/application/usecases"
)

// RankingsV2Response is the GET /api/rankings JSON body (PR-096 additive fields).
type RankingsV2Response struct {
	Timeframe     string                   `json:"timeframe"`
	Sort          string                   `json:"sort"`          // effective sort
	RequestedSort string                   `json:"requestedSort"` // query sort before fallback
	RSAvailable   bool                     `json:"rsAvailable"`
	Page          int                      `json:"page"`
	PageSize      int                      `json:"pageSize"`
	TotalItems    int                      `json:"totalItems"`
	TotalPages    int                      `json:"totalPages"`
	Precision     int                      `json:"precision"`
	Results       []RankedResultV2Response `json:"results"`
}

// RankedResultV2Response is one rankings row. rs/beta/rsRank omitted when unset.
type RankedResultV2Response struct {
	Symbol             string             `json:"symbol"`
	TotalScore         float64            `json:"totalScore"`
	Percentile         float64            `json:"percentile"`
	Scores             map[string]float64 `json:"scores"`
	Volume             float64            `json:"volume"`
	Sparkline          []float64          `json:"sparkline"`
	TrendPercentile    float64            `json:"trendPercentile"`
	SidewaysPercentile float64            `json:"sidewaysPercentile"`
	GainPercentile     float64            `json:"gainPercentile"`
	MaxPercentile      float64            `json:"maxPercentile"`
	DominantComponent  string             `json:"dominantComponent"`
	BadgeComponent     string             `json:"badgeComponent"`
	RelativeStrength   *float64           `json:"rs,omitempty"`
	Beta               *float64           `json:"beta,omitempty"`
	RSRank             *float64           `json:"rsRank,omitempty"`
}

// RankedResultToV2 maps a use-case row to the HTTP DTO.
func RankedResultToV2(r usecases.RankedResult) RankedResultV2Response {
	return RankedResultV2Response{
		Symbol:             r.Symbol.String(),
		TotalScore:         r.TotalScore,
		Percentile:         r.Percentile,
		Scores:             r.Scores,
		Volume:             r.Volume,
		Sparkline:          r.Sparkline,
		TrendPercentile:    r.TrendPercentile,
		SidewaysPercentile: r.SidewaysPercentile,
		GainPercentile:     r.GainPercentile,
		MaxPercentile:      r.MaxPercentile,
		DominantComponent:  r.DominantComponent,
		BadgeComponent:     r.BadgeComponent,
		RelativeStrength:   r.RelativeStrength,
		Beta:               r.Beta,
		RSRank:             r.RSRank,
	}
}

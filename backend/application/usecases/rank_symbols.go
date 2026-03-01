package usecases

import (
	"fmt"
	"pano_chart/backend/domain"
	"pano_chart/backend/domain/scoring"
	"sort"
)

// RankSymbols ranks symbols based on configured scoring strategies.
type RankSymbols interface {
	Rank(series map[domain.Symbol]domain.CandleSeries) ([]RankedSymbol, error)
}

type RankedSymbol struct {
	Symbol     domain.Symbol
	Scores     map[string]float64
	TotalScore float64
}

type ScoreWeight struct {
	Calculator scoring.SymbolScoreCalculator
	Weight     float64
}

// DefaultRankSymbols implements RankSymbols with explicit weights.
type DefaultRankSymbols struct {
	Weights []ScoreWeight
}

func NewDefaultRankSymbols(weights []ScoreWeight) *DefaultRankSymbols {
	return &DefaultRankSymbols{Weights: weights}
}

func (r *DefaultRankSymbols) Rank(series map[domain.Symbol]domain.CandleSeries) ([]RankedSymbol, error) {
	if len(series) == 0 {
		return nil, nil
	}

	result := make([]RankedSymbol, 0, len(series))
	for symbol, candles := range series {
		scores := make(map[string]float64)
		total := 0.0
		for _, w := range r.Weights {
			if w.Weight == 0 {
				continue
			}
			score, err := w.Calculator.Score(candles)
			if err != nil {
				return nil, fmt.Errorf("calculator %s failed for %s: %w", w.Calculator.Name(), symbol.String(), err)
			}
			scores[w.Calculator.Name()] = score
			total += score * w.Weight
		}
		result = append(result, RankedSymbol{
			Symbol:     symbol,
			Scores:     scores,
			TotalScore: total,
		})
		fmt.Printf("[RankSymbols] Calculated scores for %s: %+v, total: %.4f\n", symbol.String(), scores, total)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].TotalScore == result[j].TotalScore {
			return result[i].Symbol.String() < result[j].Symbol.String()
		}
		return result[i].TotalScore > result[j].TotalScore
	})
	return result, nil
}

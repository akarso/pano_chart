package usecases

import (
	"time"

	"github.com/akarso/pano_chart/backend/application/ports"
	"github.com/akarso/pano_chart/backend/domain"
)

// GetCandleSeries defines the use case interface.
type GetCandleSeries interface {
	Execute(symbol domain.Symbol, tf domain.Timeframe, from time.Time, to time.Time) (domain.CandleSeries, error)
}

// getCandleSeries is the concrete implementation of the use case.
type getCandleSeries struct {
	repo ports.CandleRepositoryPort
}

// NewGetCandleSeries constructs the use case with injected dependencies.
func NewGetCandleSeries(repo ports.CandleRepositoryPort) GetCandleSeries {
	return &getCandleSeries{repo: repo}
}

// Execute delegates retrieval to the CandleRepositoryPort and returns the result unchanged.
func (g *getCandleSeries) Execute(symbol domain.Symbol, tf domain.Timeframe, from time.Time, to time.Time) (domain.CandleSeries, error) {
	return g.repo.GetSeries(symbol, tf, from, to)
}

package domain

import (
	"fmt"
	"sort"
)

// CandleSeries is an ordered, immutable collection of Candle value objects.
// It represents a time-aligned, gap-aware sequence for a single Symbol and Timeframe.
type CandleSeries struct {
	symbol   Symbol
	tf       Timeframe
	candles  []Candle
	isSorted bool
}

// NewCandleSeries creates a CandleSeries from a slice of candles.
// All candles must share the same Symbol and Timeframe.
// Duplicate timestamps are rejected.
// Candles are sorted by timestamp in ascending order.
func NewCandleSeries(symbol Symbol, tf Timeframe, candles []Candle) (CandleSeries, error) {
	// Validate that all candles share the same symbol and timeframe
	for _, c := range candles {
		if c.Symbol() != symbol {
			return CandleSeries{}, fmt.Errorf("candle symbol %v does not match series symbol %v", c.Symbol(), symbol)
		}
		if c.Timeframe() != tf {
			return CandleSeries{}, fmt.Errorf("candle timeframe %v does not match series timeframe %v", c.Timeframe(), tf)
		}
	}

	// Check for duplicate timestamps
	if len(candles) > 0 {
		timestamps := make(map[string]bool)
		for _, c := range candles {
			key := c.Timestamp().String()
			if timestamps[key] {
				return CandleSeries{}, fmt.Errorf("duplicate timestamp: %v", c.Timestamp())
			}
			timestamps[key] = true
		}
	}

	// Make a defensive copy and sort by timestamp
	sortedCandles := make([]Candle, len(candles))
	copy(sortedCandles, candles)

	sort.Slice(sortedCandles, func(i, j int) bool {
		return sortedCandles[i].Timestamp().Before(sortedCandles[j].Timestamp())
	})

	return CandleSeries{
		symbol:   symbol,
		tf:       tf,
		candles:  sortedCandles,
		isSorted: true,
	}, nil
}

// Len returns the number of candles in the series.
func (cs CandleSeries) Len() int {
	return len(cs.candles)
}

// At returns the candle at the given index or an error if out of bounds.
func (cs CandleSeries) At(index int) (Candle, error) {
	if index < 0 || index >= len(cs.candles) {
		return Candle{}, fmt.Errorf("index %d out of bounds (series length: %d)", index, len(cs.candles))
	}
	return cs.candles[index], nil
}

// First returns the earliest candle in the series or an error if the series is empty.
func (cs CandleSeries) First() (Candle, error) {
	if len(cs.candles) == 0 {
		return Candle{}, fmt.Errorf("cannot get first candle from empty series")
	}
	return cs.candles[0], nil
}

// Last returns the latest candle in the series or an error if the series is empty.
func (cs CandleSeries) Last() (Candle, error) {
	if len(cs.candles) == 0 {
		return Candle{}, fmt.Errorf("cannot get last candle from empty series")
	}
	return cs.candles[len(cs.candles)-1], nil
}

// All returns a defensive copy of all candles in the series, ordered by timestamp.
func (cs CandleSeries) All() []Candle {
	if len(cs.candles) == 0 {
		return []Candle{}
	}
	copySlice := make([]Candle, len(cs.candles))
	copy(copySlice, cs.candles)
	return copySlice
}

// HasGapAfter returns true if there is a gap between the candle at index and the next candle.
// A gap exists when next.timestamp != current.timestamp + timeframe.duration.
func (cs CandleSeries) HasGapAfter(index int) bool {
	if index < 0 || index >= len(cs.candles)-1 {
		return false // No candle after this index
	}

	current := cs.candles[index]
	next := cs.candles[index+1]

	expectedNextTimestamp := current.Timestamp().Add(cs.tf.Duration())
	return !next.Timestamp().Equal(expectedNextTimestamp)
}

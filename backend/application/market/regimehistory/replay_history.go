package regimehistory

import (
	"time"

	"pano_chart/backend/domain"
	mkt "pano_chart/backend/domain/market"
)

const historyOverfetchMin = 10

// historyFetchLimit returns how many periods to load before asOf filtering.
func historyFetchLimit(limit int) int {
	if limit <= 0 {
		limit = 50
	}
	fetch := limit * 3
	if fetch < limit+historyOverfetchMin {
		fetch = limit + historyOverfetchMin
	}
	return fetch
}

// historyAtAsOf builds a point-in-time regime timeline: closed predecessors
// plus the period covering asOf shown as open with age measured at asOf.
func historyAtAsOf(periods []mkt.RegimePeriod, timeframe string, asOfUnix int64, limit int) ([]mkt.RegimePeriod, int) {
	if len(periods) == 0 {
		return nil, 0
	}
	coverIdx := -1
	for i := len(periods) - 1; i >= 0; i-- {
		p := periods[i]
		if asOfUnix < p.StartTimestamp {
			continue
		}
		if p.EndTimestamp != nil && asOfUnix >= *p.EndTimestamp {
			continue
		}
		coverIdx = i
		break
	}
	if coverIdx < 0 {
		// Gap / asOf before earliest covering period — closed periods finished by asOf.
		out := periodsEndingAtOrBefore(periods, asOfUnix)
		if len(out) > limit {
			out = out[len(out)-limit:]
		}
		age := 0
		if len(out) > 0 {
			age = out[len(out)-1].DurationCandles
		}
		return out, age
	}

	out := make([]mkt.RegimePeriod, 0, coverIdx+1)
	if coverIdx > 0 {
		out = append(out, periods[:coverIdx]...)
	}
	covering := periods[coverIdx]
	covering.EndTimestamp = nil
	covering.DurationCandles = ageCandlesAt(timeframe, covering.StartTimestamp, asOfUnix)
	out = append(out, covering)

	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, out[len(out)-1].DurationCandles
}

// periodsEndingAtOrBefore keeps closed periods with EndTimestamp ≤ asOfUnix
// (RegimeAt's exclusive end: End == asOf means the period no longer covers asOf).
func periodsEndingAtOrBefore(periods []mkt.RegimePeriod, asOfUnix int64) []mkt.RegimePeriod {
	out := make([]mkt.RegimePeriod, 0, len(periods))
	for _, p := range periods {
		if p.EndTimestamp == nil {
			continue
		}
		if *p.EndTimestamp <= asOfUnix {
			out = append(out, p)
		}
	}
	return out
}

func regimeCandleBoundary(timeframe string, ts int64) int64 {
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return ts
	}
	d := tf.Duration()
	if d == 0 {
		return ts
	}
	return time.Unix(ts, 0).UTC().Truncate(d).Unix()
}

// ageCandlesAt counts candle boundaries from period start through asOf
// (inclusive), matching tracker boundary semantics.
func ageCandlesAt(timeframe string, startUnix, asOfUnix int64) int {
	if asOfUnix < startUnix {
		return 0
	}
	tf, err := domain.NewTimeframe(timeframe)
	if err != nil {
		return 1
	}
	d := tf.Duration()
	if d == 0 {
		return 1
	}
	startB := regimeCandleBoundary(timeframe, startUnix)
	asOfB := regimeCandleBoundary(timeframe, asOfUnix)
	sec := int(asOfB-startB) / int(d.Seconds())
	if sec < 0 {
		return 0
	}
	return sec + 1
}

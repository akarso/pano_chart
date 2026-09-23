package market

// RegimeAt returns the regime covering unix timestamp `at`, if any.
//
// Intervals are half-open [Start, End): a closed period's EndTimestamp is
// exclusive so Tracker’s CloseCurrent(ts)+Append(Start=ts) boundary belongs to
// the new regime. Open periods (EndTimestamp == nil) cover [Start, +∞).
func RegimeAt(periods []RegimePeriod, at int64) (Regime, bool) {
	for _, p := range periods {
		if at < p.StartTimestamp {
			continue
		}
		if p.EndTimestamp == nil {
			return p.Regime, true
		}
		if at < *p.EndTimestamp {
			return p.Regime, true
		}
	}
	return "", false
}

package transition

import "fmt"

// candleMinutes maps known timeframe strings to their duration in minutes.
var candleMinutes = map[string]int{
	"1m":  1,
	"5m":  5,
	"15m": 15,
	"30m": 30,
	"1h":  60,
	"2h":  120,
	"4h":  240,
	"8h":  480,
	"12h": 720,
	"1d":  1440,
	"1w":  10080,
}

// HumanDuration converts a candle count at the given timeframe into a
// human-readable duration string like "20h", "3.5d", or "2w".
// Returns "" if the timeframe is unknown.
func HumanDuration(timeframe string, candles int) string {
	minutes, ok := candleMinutes[timeframe]
	if !ok || candles <= 0 {
		return ""
	}
	total := minutes * candles
	switch {
	case total < 60:
		return fmt.Sprintf("%dm", total)
	case total < 1440:
		h := float64(total) / 60
		if h == float64(int(h)) {
			return fmt.Sprintf("%dh", int(h))
		}
		return fmt.Sprintf("%.1fh", h)
	case total < 10080:
		d := float64(total) / 1440
		if d == float64(int(d)) {
			return fmt.Sprintf("%dd", int(d))
		}
		return fmt.Sprintf("%.1fd", d)
	default:
		w := float64(total) / 10080
		if w == float64(int(w)) {
			return fmt.Sprintf("%dw", int(w))
		}
		return fmt.Sprintf("%.1fw", w)
	}
}

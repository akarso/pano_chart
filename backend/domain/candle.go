package domain

import (
	"fmt"
	"time"
)

// Candle represents a single OHLCV datapoint for a given timeframe and symbol.
// It is a value object: immutable, validated at construction.
type Candle struct {
	symbol    Symbol
	timeframe Timeframe
	timestamp time.Time
	open      float64
	high      float64
	low       float64
	close     float64
	volume    float64
}

type alignRule struct {
	secondZero bool
	minuteMod  int
	minuteZero bool
	hourMod    int
	hourZero   bool
}

// NewCandle constructs a Candle and enforces invariants.
func NewCandle(symbol Symbol, tf Timeframe, ts time.Time, open, high, low, close, volume float64) (Candle, error) {
	// Validate basic invariants via helper functions to reduce cyclomatic complexity
	if err := validateTimestampUTC(ts); err != nil {
		return Candle{}, err
	}
	if err := validateNonNegative(open, high, low, close, volume); err != nil {
		return Candle{}, err
	}
	if err := validatePriceInvariants(open, high, low, close); err != nil {
		return Candle{}, err
	}
	if err := validateTemporalAlignment(tf, ts); err != nil {
		return Candle{}, err
	}

	c := Candle{
		symbol:    symbol,
		timeframe: tf,
		timestamp: ts,
		open:      open,
		high:      high,
		low:       low,
		close:     close,
		volume:    volume,
	}
	return c, nil
}

func validateTimestampUTC(ts time.Time) error {
	if ts.Location() != time.UTC {
		return fmt.Errorf("timestamp must be in UTC")
	}
	return nil
}

func validateNonNegative(vals ...float64) error {
	for _, v := range vals {
		if v < 0 {
			return fmt.Errorf("prices and volume must be non-negative")
		}
	}
	return nil
}

func validatePriceInvariants(open, high, low, close float64) error {
	if high < open || high < close {
		return fmt.Errorf("high must be >= max(open, close)")
	}
	if low > open || low > close {
		return fmt.Errorf("low must be <= min(open, close)")
	}
	if high < low {
		return fmt.Errorf("high must be >= low")
	}
	return nil
}

func validateTemporalAlignment(tf Timeframe, ts time.Time) error {
	rules := map[Timeframe]alignRule{
		Timeframe1m:  {secondZero: true},
		Timeframe5m:  {secondZero: true, minuteMod: 5},
		Timeframe15m: {secondZero: true, minuteMod: 15},
		Timeframe1h:  {secondZero: true, minuteZero: true},
		Timeframe4h:  {secondZero: true, minuteZero: true, hourMod: 4},
		Timeframe1d:  {secondZero: true, minuteZero: true, hourZero: true},
	}
	rule, ok := rules[tf]
	if !ok {
		return fmt.Errorf("unsupported timeframe: %v", tf)
	}
	if err := checkSecond(rule, tf, ts); err != nil {
		return err
	}
	if err := checkMinute(rule, tf, ts); err != nil {
		return err
	}
	if err := checkHour(rule, tf, ts); err != nil {
		return err
	}
	return nil
}

func checkSecond(rule alignRule, tf Timeframe, ts time.Time) error {
	if rule.secondZero && ts.Second() != 0 {
		return fmt.Errorf("%v timeframe requires second == 0", tf)
	}
	return nil
}

func checkMinute(rule alignRule, tf Timeframe, ts time.Time) error {
	if rule.minuteZero && ts.Minute() != 0 {
		return fmt.Errorf("%v timeframe requires minute == 0", tf)
	}
	if rule.minuteMod != 0 && ts.Minute()%rule.minuteMod != 0 {
		return fmt.Errorf("%v timeframe requires minute divisible by %d", tf, rule.minuteMod)
	}
	return nil
}

func checkHour(rule alignRule, tf Timeframe, ts time.Time) error {
	if rule.hourZero && ts.Hour() != 0 {
		return fmt.Errorf("%v timeframe requires hour == 0", tf)
	}
	if rule.hourMod != 0 && ts.Hour()%rule.hourMod != 0 {
		return fmt.Errorf("%v timeframe requires hour divisible by %d", tf, rule.hourMod)
	}
	return nil
}

// Accessors
func (c Candle) Symbol() Symbol { return c.symbol }
func (c Candle) Timeframe() Timeframe { return c.timeframe }
func (c Candle) Timestamp() time.Time { return c.timestamp }
func (c Candle) Open() float64 { return c.open }
func (c Candle) High() float64 { return c.high }
func (c Candle) Low() float64 { return c.low }
func (c Candle) Close() float64 { return c.close }
func (c Candle) Volume() float64 { return c.volume }

// Identity equality: symbol + timeframe + timestamp
func (c Candle) Equals(other Candle) bool {
	if c.symbol != other.symbol {
		return false
	}
	if c.timeframe != other.timeframe {
		return false
	}
	return c.timestamp.Equal(other.timestamp)
}

// Derived properties
func (c Candle) IsBullish() bool { return c.close > c.open }
func (c Candle) IsBearish() bool { return c.close < c.open }
func (c Candle) IsDoji() bool    { return c.close == c.open }

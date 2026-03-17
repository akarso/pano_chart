package behavior

// RetailBehavior represents the interpreted behavioral dimensions of
// current market positioning and flow.
type RetailBehavior struct {
	Symbol    string
	Timeframe string

	Greed    float64
	Fear     float64
	Patience float64
	Panic    float64

	Summary string
}

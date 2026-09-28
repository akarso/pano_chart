package metrics

// CompositeTapeWindow is the CalculateTape bar limit shared by Market Pulse
// (MarketStateService) and rankings relative strength. One value keeps the
// Redis key market_composite_v3:tape:{tf}:{n} aligned and avoids a second
// composite fan-out.
const CompositeTapeWindow = 110

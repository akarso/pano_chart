package candle_repository

import (
	"pano_chart/backend/domain"
)

func timeframeToMinutes(tf domain.Timeframe) (int, error) {
	switch tf {
	case domain.Timeframe15m:
		return 15, nil
	case domain.Timeframe1h:
		return 60, nil
	case domain.Timeframe4h:
		return 240, nil
	case domain.Timeframe1d:
		return 1440, nil
	default:
		return 0, ErrUnsupportedTimeframe
	}
}

package transition

import (
	"pano_chart/backend/application/market/regimehistory"
	mkt "pano_chart/backend/domain/market"
)

// HistoryFromService adapts regimehistory.Service to PeriodHistory.
type HistoryFromService struct {
	Service *regimehistory.Service
}

// GetHistory implements PeriodHistory.
func (h HistoryFromService) GetHistory(timeframe string, limit int) ([]mkt.RegimePeriod, error) {
	if h.Service == nil {
		return nil, nil
	}
	hist, err := h.Service.GetHistory(timeframe, limit)
	if err != nil {
		return nil, err
	}
	return hist.Periods, nil
}

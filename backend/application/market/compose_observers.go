package market

import mkt "pano_chart/backend/domain/market"

// ComposeObservers fans Update out to all non-nil observers. First error is
// returned after all have been attempted (best-effort fan-out).
func ComposeObservers(observers ...RegimeObserver) RegimeObserver {
	var list []RegimeObserver
	for _, o := range observers {
		if o != nil {
			list = append(list, o)
		}
	}
	switch len(list) {
	case 0:
		return nil
	case 1:
		return list[0]
	default:
		return multiObserver(list)
	}
}

type multiObserver []RegimeObserver

func (m multiObserver) Update(timeframe string, regime mkt.Regime, bias string, timestamp int64) error {
	var first error
	for _, o := range m {
		if err := o.Update(timeframe, regime, bias, timestamp); err != nil && first == nil {
			first = err
		}
	}
	return first
}

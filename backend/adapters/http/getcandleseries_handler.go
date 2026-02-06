package http

import (
	"encoding/json"
	"net/http"
	"time"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

// NewGetCandleSeriesHandler constructs an http.HandlerFunc that adapts HTTP requests
// to the GetCandleSeries use case.
func NewGetCandleSeriesHandler(uc usecases.GetCandleSeries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		symStr := q.Get("symbol")
		tfStr := q.Get("timeframe")
		fromStr := q.Get("from")
		toStr := q.Get("to")

		if symStr == "" || tfStr == "" || fromStr == "" || toStr == "" {
			http.Error(w, "missing required query parameters", http.StatusBadRequest)
			return
		}

		// Construct domain objects
		sym, err := domain.NewSymbol(symStr)
		if err != nil {
			http.Error(w, "invalid symbol", http.StatusBadRequest)
			return
		}
		tf, err := domain.NewTimeframe(tfStr)
		if err != nil {
			http.Error(w, "invalid timeframe", http.StatusBadRequest)
			return
		}

		from, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			http.Error(w, "invalid from time", http.StatusBadRequest)
			return
		}
		if from.Location() != time.UTC {
			from = from.UTC()
		}
		to, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			http.Error(w, "invalid to time", http.StatusBadRequest)
			return
		}
		if to.Location() != time.UTC {
			to = to.UTC()
		}

		series, err := uc.Execute(sym, tf, from, to)
		if err != nil {
			http.Error(w, "use case error", http.StatusInternalServerError)
			return
		}

		// Build response
		resp := struct {
			Symbol    string `json:"symbol"`
			Timeframe string `json:"timeframe"`
			Candles   []struct {
				Timestamp string  `json:"timestamp"`
				Open      float64 `json:"open"`
				High      float64 `json:"high"`
				Low       float64 `json:"low"`
				Close     float64 `json:"close"`
				Volume    float64 `json:"volume"`
			} `json:"candles"`
		}{
			Symbol:    sym.String(),
			Timeframe: tf.String(),
		}

		all := series.All()
		resp.Candles = make([]struct {
			Timestamp string  `json:"timestamp"`
			Open      float64 `json:"open"`
			High      float64 `json:"high"`
			Low       float64 `json:"low"`
			Close     float64 `json:"close"`
			Volume    float64 `json:"volume"`
		}, len(all))

		for i, c := range all {
			resp.Candles[i].Timestamp = c.Timestamp().Format(time.RFC3339)
			resp.Candles[i].Open = c.Open()
			resp.Candles[i].High = c.High()
			resp.Candles[i].Low = c.Low()
			resp.Candles[i].Close = c.Close()
			resp.Candles[i].Volume = c.Volume()
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

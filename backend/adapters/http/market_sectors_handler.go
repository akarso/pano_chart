package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"pano_chart/backend/application/market/metrics"
	"pano_chart/backend/domain"
)

// MarketSectorsHandler handles GET /api/market/sectors.
type MarketSectorsHandler struct {
	service metrics.SectorsCalculator
}

// NewMarketSectorsHandler constructs the handler.
func NewMarketSectorsHandler(s metrics.SectorsCalculator) *MarketSectorsHandler {
	return &MarketSectorsHandler{service: s}
}

type sectorsResponse struct {
	Timeframe         string           `json:"timeframe"`
	MarketSymbolCount int              `json:"marketSymbolCount"`
	Sectors           []sectorIndexDTO `json:"sectors"`
}

type sectorIndexDTO struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	SymbolCount int             `json:"symbolCount"`
	Points      []indexPointDTO `json:"points"`
	Return      float64         `json:"return"`
	RS          float64         `json:"rs"`
	RSAvailable bool            `json:"rsAvailable"`
}

// ServeHTTP implements http.Handler.
func (h *MarketSectorsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tfStr := r.URL.Query().Get("timeframe")
	if tfStr == "" {
		tfStr = "4h"
	}
	tf, err := domain.NewTimeframe(tfStr)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid timeframe"}`))
		return
	}

	limit := 200
	if s := r.URL.Query().Get("limit"); s != "" {
		v, convErr := strconv.Atoi(s)
		if convErr != nil || v <= 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid limit"}`))
			return
		}
		if v > 500 {
			v = 500
		}
		limit = v
	}

	result, err := h.service.Calculate(r.Context(), tf.String(), limit)
	if err != nil {
		log.Printf("[sectors] calculate failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"sectors unavailable"}`))
		return
	}

	sectors := make([]sectorIndexDTO, len(result.Sectors))
	for i, sec := range result.Sectors {
		pts := make([]indexPointDTO, len(sec.Points))
		for j, p := range sec.Points {
			pts[j] = indexPointDTO{
				T: p.Timestamp,
				V: roundTo(p.Value, 6),
			}
		}
		sectors[i] = sectorIndexDTO{
			ID:          sec.ID,
			Name:        sec.Name,
			SymbolCount: sec.SymbolCount,
			Points:      pts,
			Return:      roundTo(sec.Return, 6),
			RS:          roundTo(sec.RS, 6),
			RSAvailable: sec.RSAvailable,
		}
	}

	resp := sectorsResponse{
		Timeframe:         result.Timeframe,
		MarketSymbolCount: result.MarketSymbolCount,
		Sectors:           sectors,
	}
	if resp.Sectors == nil {
		resp.Sectors = []sectorIndexDTO{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

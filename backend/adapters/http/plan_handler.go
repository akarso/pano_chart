package http

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"pano_chart/backend/application/plan"
	"pano_chart/backend/domain"
)

const planSuffix = "/plan"

// PlanEvaluator builds a range plan for a symbol/timeframe.
type PlanEvaluator interface {
	Evaluate(ctx context.Context, symbol, timeframe string, risk float64) (plan.PlanResult, error)
}

// PlanHandler handles GET /api/symbol/{symbol}/plan?timeframe=&risk=.
type PlanHandler struct {
	svc PlanEvaluator
}

// NewPlanHandler constructs the handler.
func NewPlanHandler(svc PlanEvaluator) *PlanHandler {
	return &PlanHandler{svc: svc}
}

func (h *PlanHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		writeError(w, http.StatusServiceUnavailable, "PLAN_UNAVAILABLE", "plan endpoint not configured")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "only GET is supported")
		return
	}
	symbolStr, ok := extractPlanSymbol(r.URL.Path)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", "expected /api/symbol/{symbol}/plan")
		return
	}
	sym, err := ParseSymbol(symbolStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_SYMBOL", "invalid symbol")
		return
	}
	tf := r.URL.Query().Get("timeframe")
	if tf == "" {
		tf = "1h"
	}
	if _, err := domain.NewTimeframe(tf); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_TIMEFRAME", "invalid timeframe")
		return
	}
	risk := 0.0
	if rs := r.URL.Query().Get("risk"); rs != "" {
		risk, err = strconv.ParseFloat(rs, 64)
		if err != nil || risk < 0 || math.IsNaN(risk) || math.IsInf(risk, 0) {
			writeError(w, http.StatusBadRequest, "INVALID_RISK", "risk must be a non-negative number")
			return
		}
	}

	result, err := h.svc.Evaluate(r.Context(), sym.String(), tf, risk)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			writeError(w, statusClientClosedRequest, "CLIENT_CLOSED", "client closed request")
		case errors.Is(err, context.DeadlineExceeded):
			writeError(w, http.StatusGatewayTimeout, "DEADLINE_EXCEEDED", "request deadline exceeded")
		case errors.Is(err, plan.ErrDataUnavailable):
			writeError(w, http.StatusUnprocessableEntity, "DATA_UNAVAILABLE", "candle data unavailable")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, planDTOFrom(result))
}

func extractPlanSymbol(path string) (string, bool) {
	if !strings.HasPrefix(path, symbolDetailPrefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, symbolDetailPrefix)
	if !strings.HasSuffix(rest, planSuffix) {
		return "", false
	}
	symbol := strings.TrimSuffix(rest, planSuffix)
	if symbol == "" || strings.Contains(symbol, "/") {
		return "", false
	}
	return symbol, true
}

type planDTO struct {
	Symbol          string  `json:"symbol"`
	Timeframe       string  `json:"timeframe"`
	Low             float64 `json:"low"`
	High            float64 `json:"high"`
	Mid             float64 `json:"mid"`
	ATR             float64 `json:"atr"`
	Price           float64 `json:"price"`
	LongEntry       float64 `json:"longEntry"`
	LongStop        float64 `json:"longStop"`
	LongTarget      float64 `json:"longTarget"`
	LongTargetFull  float64 `json:"longTargetFull"`
	ShortEntry      float64 `json:"shortEntry"`
	ShortStop       float64 `json:"shortStop"`
	ShortTarget     float64 `json:"shortTarget"`
	ShortTargetFull float64 `json:"shortTargetFull"`
	RiskReward      float64 `json:"riskReward"`
	RangeQuality    float64 `json:"rangeQuality"`
	Position        float64 `json:"position"`
	Valid           bool    `json:"valid"`
	Reason          string  `json:"reason,omitempty"`
	Size            float64 `json:"size"`
	ShortSize       float64 `json:"shortSize"`
}

func planDTOFrom(r plan.PlanResult) planDTO {
	p := r.Plan
	return planDTO{
		Symbol:          p.Symbol,
		Timeframe:       p.Timeframe,
		Low:             p.Low,
		High:            p.High,
		Mid:             p.Mid,
		ATR:             p.ATR,
		Price:           p.Price,
		LongEntry:       p.LongEntry,
		LongStop:        p.LongStop,
		LongTarget:      p.LongTarget,
		LongTargetFull:  p.LongTargetFull,
		ShortEntry:      p.ShortEntry,
		ShortStop:       p.ShortStop,
		ShortTarget:     p.ShortTarget,
		ShortTargetFull: p.ShortTargetFull,
		RiskReward:      p.RiskReward,
		RangeQuality:    p.RangeQuality,
		Position:        p.Position,
		Valid:           p.Valid,
		Reason:          p.Reason,
		Size:            r.Size,
		ShortSize:       r.ShortSize,
	}
}

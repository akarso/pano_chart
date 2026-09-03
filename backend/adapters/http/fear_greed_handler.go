package http

import (
	"encoding/json"
	"net/http"
	"time"

	"pano_chart/backend/application/usecases"
)

// FearGreedHandler handles GET /api/v1/fear-greed requests.
type FearGreedHandler struct {
	uc usecases.FearGreedUseCase
}

// NewFearGreedHandler constructs the handler.
func NewFearGreedHandler(uc usecases.FearGreedUseCase) *FearGreedHandler {
	return &FearGreedHandler{uc: uc}
}

// fearGreedResponse is the JSON envelope returned to the client.
type fearGreedResponse struct {
	Value               int    `json:"value"`
	ValueClassification string `json:"valueClassification"`
	TimestampUTC        string `json:"timestampUtc"`
}

// ServeHTTP implements http.Handler.
func (h *FearGreedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	result, err := h.uc.Execute(r.Context())
	if err != nil {
		http.Error(w, `{"error":"failed to fetch fear & greed index"}`, http.StatusBadGateway)
		return
	}

	resp := fearGreedResponse{
		Value:               result.Value,
		ValueClassification: result.ValueClassification,
		TimestampUTC:        time.Unix(result.Timestamp, 0).UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, `{"error":"encoding error"}`, http.StatusInternalServerError)
	}
}

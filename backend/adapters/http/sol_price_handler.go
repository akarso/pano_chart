package http

import (
	"encoding/json"
	"net/http"

	"pano_chart/backend/infrastructure/solana"
)

// SolPriceResponse is the JSON body returned by the SOL price endpoint.
type SolPriceResponse struct {
	SolPrice    float64 `json:"sol_price"`
	RequiredSOL float64 `json:"required_sol"`
	PriceUSD    float64 `json:"price_usd"`
	Wallet      string  `json:"wallet"`
}

// NewSolPriceHandler returns a handler that responds with the current
// SOL price and the required SOL amount for a subscription.
func NewSolPriceHandler(provider *solana.Provider, wallet string, priceUSD float64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		solPrice, err := provider.GetSOLPricePublic(r.Context())
		if err != nil {
			http.Error(w, "failed to fetch SOL price", http.StatusServiceUnavailable)
			return
		}

		resp := SolPriceResponse{
			SolPrice:    solPrice,
			RequiredSOL: provider.RequiredSOL(solPrice),
			PriceUSD:    priceUSD,
			Wallet:      wallet,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}
}

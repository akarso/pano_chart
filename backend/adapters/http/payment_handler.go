package http

import (
	"encoding/json"
	"net/http"
	"time"

	"pano_chart/backend/adapters/http/middleware"
	"pano_chart/backend/application/usecases"
)

// ---- Verify Purchase Handler ----

type verifyPurchaseRequest struct {
	Provider      string `json:"provider"`
	PurchaseToken string `json:"purchaseToken"`
	UserID        string `json:"userId"`
}

// NewVerifyPurchaseHandler returns an http.HandlerFunc that verifies a
// purchase token via the VerifyPurchase use case.
//
//	POST /api/payments/verify
//	Body: { "provider": "...", "purchaseToken": "...", "userId": "..." }
func NewVerifyPurchaseHandler(uc usecases.VerifyPurchase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req verifyPurchaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		input := usecases.VerifyPurchaseInput{
			Provider:      req.Provider,
			PurchaseToken: req.PurchaseToken,
			UserID:        req.UserID,
		}
		if err := uc.Execute(r.Context(), input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// ---- Subscription Status Handler ----

type subscriptionStatusResponse struct {
	Active    bool   `json:"active"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// NewSubscriptionStatusHandler returns an http.HandlerFunc that checks
// the subscription status for the authenticated caller.
//
//	GET /api/subscription/status
//	Header: Authorization: Bearer <device secret>
//
// Must be registered behind middleware.RequireAuth — the user ID comes
// from the verified secret, never from client input.
func NewSubscriptionStatusHandler(svc usecases.SubscriptionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID := middleware.UserIDFromContext(r.Context())

		sub, found, err := svc.GetSubscription(r.Context(), userID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		resp := subscriptionStatusResponse{Active: false}
		if found {
			resp.Active = sub.IsActive(time.Now().UTC())
			resp.ExpiresAt = sub.ExpirationTime().Format(time.RFC3339)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

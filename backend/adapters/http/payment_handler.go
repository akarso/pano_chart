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
//	GET /api/subscription/status[?userId=...]
//	Header: Authorization: Bearer <device secret>
//
// Must be registered behind middleware.RequireAuth. The user ID comes from
// the verified secret whenever one is present. The `?userId=` query param
// is ONLY consulted as a migration-window fallback while RequireAuth is
// running in log-only mode for a pre-PR-070 client that has no secret yet
// — once RequireAuth enforces (enforce=true), unauthenticated requests
// never reach this handler at all, so the fallback branch is dead and can
// be deleted then.
func NewSubscriptionStatusHandler(svc usecases.SubscriptionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID, ok := middleware.UserIDFromContextOK(r.Context())
		if !ok {
			userID = r.URL.Query().Get("userId")
			if userID == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

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

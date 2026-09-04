package http

import (
	"encoding/json"
	"net/http"
	"time"

	"pano_chart/backend/adapters/http/middleware"
	"pano_chart/backend/application/ports"
	"pano_chart/backend/application/usecases"
)

// ---- Verify Purchase Handler ----

type verifyPurchaseRequest struct {
	Provider      string `json:"provider"`
	PurchaseToken string `json:"purchaseToken"`
}

// NewVerifyPurchaseHandler returns an http.HandlerFunc that verifies a
// purchase token and activates a subscription for the authenticated
// caller.
//
//	POST /api/payments/verify
//	Header: Authorization: Bearer <device secret>
//	Body: { "provider": "...", "purchaseToken": "..." }
//
// Unlike every other PR-070 endpoint, this one has NO migration-window
// fallback to a client-supplied identity — a wrong `userId` here means an
// attacker-chosen account gets a paid subscription for free, not just a
// data leak, so the grace period bought for other routes isn't worth
// extending here. Use NewVerifyPurchaseRoute to wire this with
// hard-enforced auth (independent of the general AUTH_ENFORCE flag) —
// don't register this handler directly behind the shared log-only
// middleware.
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

		// Panics (not UserIDFromContextOK) if this handler is ever reached
		// without a verified secret — see NewVerifyPurchaseRoute; there is
		// deliberately no fallback identity to catch a wiring mistake here.
		userID := middleware.UserIDFromContext(r.Context())

		input := usecases.VerifyPurchaseInput{
			Provider:      req.Provider,
			PurchaseToken: req.PurchaseToken,
			UserID:        userID,
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

// verifyPurchaseRateLimitPerHour / Burst bound how often one authenticated
// user can hit /api/payments/verify. Each call triggers a live provider API
// request (Google Play/App Store) with a real cost, and — unlike outbound
// exchange calls, already throttled via infrastructure/ratelimiter — this
// endpoint had no abuse protection at all before PR-075. Burst equals the
// hourly limit so a legitimate user isn't blocked retrying a few times in
// quick succession after a flaky provider response, but sustained
// hammering past 5/hour is rejected.
const (
	verifyPurchaseRateLimitPerHour = 5
	verifyPurchaseRateLimitBurst   = 5
)

// NewVerifyPurchaseRoute wires the production handler chain for
// POST /api/payments/verify: auth is HARD-enforced (enforce=true) here,
// independent of the general AUTH_ENFORCE env var that the other PR-070
// endpoints share — this is the one route where the financial blast
// radius of a false negative (free subscription) outweighs the migration
// grace period. cmd/api/main.go must wire this route through this
// constructor, not by hand-assembling middleware.RequireAuth(..., false)
// or authMW — see the router-level test in payment_handler_test.go, which
// calls this exact function to catch that class of regression.
//
// Rate limiting (PR-075) is the innermost layer, applied to the handler
// before RequireAuth wraps it — PerUserRateLimit reads the authenticated
// user ID from context, which only exists once RequireAuth has already run.
func NewVerifyPurchaseRoute(uc usecases.VerifyPurchase, store ports.CredentialStore) http.Handler {
	limited := middleware.PerUserRateLimit(verifyPurchaseRateLimitPerHour, verifyPurchaseRateLimitBurst)(NewVerifyPurchaseHandler(uc))
	return middleware.RequireAuth(store, true)(limited)
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

		userID, ok := middleware.UserIDOrLegacyFallback(r.Context(), r.URL.Query().Get("userId"))
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
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

package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"

	"pano_chart/backend/application/ports"
	"pano_chart/backend/application/replay"
)

// ReplaySubscriptionChecker reports whether a user has an active Pro
// subscription. Same shape as notifications.SubscriptionChecker.
type ReplaySubscriptionChecker interface {
	IsActive(ctx context.Context, userID string) (bool, error)
}

const (
	// ReplayRateLimitPerMinute is the per-user budget for asOf requests (PR-112a).
	ReplayRateLimitPerMinute = 10
	// ReplayRateLimitBurst allows a short scrubber burst within the minute budget.
	ReplayRateLimitBurst = 5
)

// RequireReplayAccess gates requests that carry ?asOf=:
//   - parses and attaches replay.WithAsOf
//   - hard-requires Bearer auth (independent of AUTH_ENFORCE)
//   - rate-limits at ReplayRateLimitPerMinute / user (before entitlement I/O)
//   - requires an active Pro subscription when a checker is configured
//
// Requests without asOf pass through unchanged (no auth / no rate limit).
func RequireReplayAccess(
	creds ports.CredentialStore,
	subs ReplaySubscriptionChecker,
) func(http.Handler) http.Handler {
	limiter := perKeyRateLimit(func(r *http.Request) string {
		userID, ok := UserIDFromContextOK(r.Context())
		if !ok {
			return ""
		}
		return "replay:" + userID
	}, float64(ReplayRateLimitPerMinute)/60.0, ReplayRateLimitBurst)

	return func(next http.Handler) http.Handler {
		// Rate-limit wraps entitlement + handler so exhausted callers are
		// rejected cheaply without hitting the subscription store.
		limited := limiter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := UserIDFromContextOK(r.Context())
			if !ok {
				writeReplayError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if subs != nil {
				active, serr := subs.IsActive(r.Context(), userID)
				if serr != nil {
					log.Printf("[replay] subscription check error user=%s: %v", userID, serr)
					writeReplayError(w, http.StatusServiceUnavailable, "subscription check failed")
					return
				}
				if !active {
					writeReplayError(w, http.StatusForbidden, "pro required")
					return
				}
			}
			next.ServeHTTP(w, r)
		}))

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.URL.Query().Get("asOf")
			asOf, err := replay.ParseUnixSeconds(raw)
			if err != nil {
				writeReplayError(w, http.StatusBadRequest, "invalid asOf")
				return
			}
			if asOf == nil {
				next.ServeHTTP(w, r)
				return
			}
			if err := replay.ValidateAsOf(*asOf, time.Now().UTC()); err != nil {
				writeReplayError(w, http.StatusBadRequest, "invalid asOf")
				return
			}

			userID, ok := lookupBearerUser(r, creds)
			if !ok {
				writeReplayError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			ctx := WithUserID(r.Context(), userID)
			ctx = replay.WithAsOf(ctx, *asOf)
			limited.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeReplayError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}

func lookupBearerUser(r *http.Request, store ports.CredentialStore) (string, bool) {
	if store == nil {
		return "", false
	}
	auth := r.Header.Get("Authorization")
	secret, hasPrefix := strings.CutPrefix(auth, "Bearer ")
	if !hasPrefix || secret == "" {
		return "", false
	}
	hash := sha256.Sum256([]byte(secret))
	userID, ok, err := store.Lookup(r.Context(), hex.EncodeToString(hash[:]))
	if err != nil || !ok {
		return "", false
	}
	return userID, true
}

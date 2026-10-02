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
//   - requires an active Pro subscription when a checker is configured
//   - rate-limits at ReplayRateLimitPerMinute / user (asOf traffic only)
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
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.URL.Query().Get("asOf")
			asOf, err := replay.ParseUnixSeconds(raw)
			if err != nil {
				http.Error(w, `{"error":"invalid asOf"}`, http.StatusBadRequest)
				return
			}
			if asOf == nil {
				next.ServeHTTP(w, r)
				return
			}
			if err := replay.ValidateAsOf(*asOf, time.Now().UTC()); err != nil {
				http.Error(w, `{"error":"invalid asOf"}`, http.StatusBadRequest)
				return
			}

			userID, ok := lookupBearerUser(r, creds)
			if !ok {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			ctx := WithUserID(r.Context(), userID)

			if subs != nil {
				active, serr := subs.IsActive(ctx, userID)
				if serr != nil {
					log.Printf("[replay] subscription check error user=%s: %v", userID, serr)
					http.Error(w, `{"error":"subscription check failed"}`, http.StatusServiceUnavailable)
					return
				}
				if !active {
					http.Error(w, `{"error":"pro required"}`, http.StatusForbidden)
					return
				}
			}

			ctx = replay.WithAsOf(ctx, *asOf)
			limited.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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

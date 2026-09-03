// Package middleware holds cross-cutting HTTP middleware for the API.
package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"pano_chart/backend/application/ports"
)

type contextKey string

const userIDContextKey contextKey = "auth.userID"

// RequireAuth returns middleware that resolves the caller's user ID from an
// `Authorization: Bearer <secret>` header (issued by POST /api/device/claim)
// and rejects the request with 401 if it's missing or unrecognised.
//
// Handlers behind this middleware must read the verified ID via
// UserIDFromContext — never trust a client-supplied userId/user_id field
// again (that was the whole point of this PR).
func RequireAuth(store ports.CredentialStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			secret, hasPrefix := strings.CutPrefix(auth, "Bearer ")
			if !hasPrefix || secret == "" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			hash := sha256.Sum256([]byte(secret))
			userID, ok, err := store.Lookup(r.Context(), hex.EncodeToString(hash[:]))
			if err != nil {
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
				return
			}
			if !ok {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), userID)))
		})
	}
}

// WithUserID returns a context carrying the given authenticated user ID.
// Exposed so tests can simulate a request that already passed through
// RequireAuth without standing up a real credential store.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey, userID)
}

// UserIDFromContext returns the authenticated user ID injected by
// RequireAuth. Panics if called on a request that didn't go through the
// middleware — that's a routing/wiring bug, not a runtime condition for
// handlers to handle gracefully. (net/http recovers per-request panics, so
// this fails the one offending request with a 500, not the whole server.)
func UserIDFromContext(ctx context.Context) string {
	userID, ok := ctx.Value(userIDContextKey).(string)
	if !ok {
		panic("middleware.UserIDFromContext: no authenticated user in context — handler not wrapped with RequireAuth")
	}
	return userID
}

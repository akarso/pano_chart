// Package middleware holds cross-cutting HTTP middleware for the API.
package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"strings"

	"pano_chart/backend/application/ports"
)

type contextKey string

const userIDContextKey contextKey = "auth.userID"

// RequireAuth returns middleware that resolves the caller's user ID from an
// `Authorization: Bearer <secret>` header (issued by POST /api/device/claim).
//
// enforce controls what happens when the header is missing/invalid:
//   - enforce=true: reject with 401. Use this once client adoption of
//     /api/device/claim is confirmed (see PR-070's rollout notes).
//   - enforce=false ("log-only" migration mode): log the miss and let the
//     request through anyway, WITHOUT a verified user ID in the context.
//     This exists so deploying this middleware doesn't immediately break
//     every install running a pre-PR-070 app build, which has no secret to
//     send yet. Handlers behind a log-only route must use
//     UserIDFromContextOK (not the panicking UserIDFromContext) and fall
//     back to whatever legacy identity field that specific endpoint used to
//     trust — see the fallback blocks in payment_handler.go,
//     device_register_handler.go, and notification_config_handler.go. Once
//     enforce flips to true, those fallback branches become dead code
//     (log-only requests can no longer reach the handler at all) and can be
//     deleted.
func RequireAuth(store ports.CredentialStore, enforce bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			secret, hasPrefix := strings.CutPrefix(auth, "Bearer ")
			if hasPrefix && secret != "" {
				hash := sha256.Sum256([]byte(secret))
				userID, ok, err := store.Lookup(r.Context(), hex.EncodeToString(hash[:]))
				if err != nil {
					http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
					return
				}
				if ok {
					next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), userID)))
					return
				}
			}

			if enforce {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			log.Printf("[auth] log-only mode: unauthenticated request allowed through: %s %s", r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
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
// middleware, OR that did but wasn't authenticated (log-only mode let it
// through anyway) — only use this on a route that's either always behind
// enforce=true, or that has no legacy fallback to fall back to (e.g.
// device unregister, which never took a client-supplied identity). Routes
// that need a migration-window fallback must use UserIDFromContextOK
// instead. (net/http recovers per-request panics, so a wrong call here
// fails the one offending request with a 500, not the whole server.)
func UserIDFromContext(ctx context.Context) string {
	userID, ok := ctx.Value(userIDContextKey).(string)
	if !ok {
		panic("middleware.UserIDFromContext: no authenticated user in context — handler not wrapped with RequireAuth, or RequireAuth is running in log-only mode without a fallback")
	}
	return userID
}

// UserIDFromContextOK returns the authenticated user ID and true if
// RequireAuth verified one for this request. Returns ("", false) when the
// request has no verified identity — either it never went through
// RequireAuth, or it did but RequireAuth is running in log-only mode
// (enforce=false) and let an unauthenticated request through. Callers must
// have their own fallback for the false case (a migration-window legacy
// field, or simply rejecting the request) — see the handlers using this.
func UserIDFromContextOK(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDContextKey).(string)
	return userID, ok
}

// UserIDOrLegacyFallback returns the authenticated user ID from context if
// RequireAuth verified one, otherwise falls back to legacyValue (a
// client-supplied field from the request body/query — the pre-PR-070
// trust model). Returns ok=false if neither is available, meaning the
// caller should reject the request.
//
// This centralizes the migration-window fallback pattern used by every
// route still behind log-only RequireAuth, so there's exactly one place to
// delete it from once AUTH_ENFORCE=true makes it permanently unreachable —
// see UserIDFromContextOK's doc for why the fallback exists at all. Do NOT
// use this for a route that must never trust an unverified identity
// regardless of enforce mode (e.g. /api/payments/verify, which is
// hard-enforced independent of AUTH_ENFORCE — see
// adapters/http.NewVerifyPurchaseRoute) — for those, use the plain
// panicking UserIDFromContext instead, so a wiring mistake fails loudly.
func UserIDOrLegacyFallback(ctx context.Context, legacyValue string) (string, bool) {
	if userID, ok := UserIDFromContextOK(ctx); ok {
		return userID, true
	}
	if legacyValue != "" {
		return legacyValue, true
	}
	return "", false
}

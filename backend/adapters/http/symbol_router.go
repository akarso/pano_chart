package http

import (
	"net/http"
	"strings"
)

// SymbolRouter dispatches /api/symbol/{symbol}[/{action}] requests: an
// action suffix (e.g. "/regimes", "/plan") routes to its handler; anything
// else (a bare symbol) falls through to the existing detail handler.
type SymbolRouter struct {
	detail  http.Handler
	regimes http.Handler
	plan    http.Handler
}

// NewSymbolRouter constructs a router for the /api/symbol/ prefix.
func NewSymbolRouter(detail, regimes http.Handler) *SymbolRouter {
	return &SymbolRouter{detail: detail, regimes: regimes}
}

// SetPlanHandler registers GET …/plan (PR-110). Nil-safe when unset.
func (r *SymbolRouter) SetPlanHandler(h http.Handler) {
	if r == nil {
		return
	}
	r.plan = h
}

// ServeHTTP dispatches to the correct sub-handler.
//
// A plain strings.HasSuffix(path, action) would misroute a bare symbol
// literally named after the action. Require the action to be its own path
// segment after the fixed prefix.
func (r *SymbolRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	rest := strings.TrimPrefix(req.URL.Path, symbolDetailPrefix)
	if idx := strings.LastIndex(rest, "/"); idx >= 0 {
		action := rest[idx:]
		switch action {
		case mtfSuffix:
			r.regimes.ServeHTTP(w, req)
			return
		case planSuffix:
			if r.plan != nil {
				r.plan.ServeHTTP(w, req)
				return
			}
			writeError(w, http.StatusServiceUnavailable, "PLAN_UNAVAILABLE", "plan endpoint not configured")
			return
		}
	}
	r.detail.ServeHTTP(w, req)
}

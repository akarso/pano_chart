package http

import (
	"net/http"
	"strings"
)

// SymbolRouter dispatches /api/symbol/{symbol}[/{action}] requests: an
// action suffix (e.g. "/regimes") routes to its handler; anything else (a
// bare symbol) falls through to the existing detail handler, preserving
// GET /api/symbol/{symbol} exactly as before PR-099.
type SymbolRouter struct {
	detail  http.Handler
	regimes http.Handler
}

// NewSymbolRouter constructs a router for the /api/symbol/ prefix.
func NewSymbolRouter(detail, regimes http.Handler) *SymbolRouter {
	return &SymbolRouter{detail: detail, regimes: regimes}
}

// ServeHTTP dispatches to the correct sub-handler.
//
// A plain strings.HasSuffix(path, mtfSuffix) would misroute a bare symbol
// literally named "regimes" (path "/api/symbol/regimes"): the whole
// remainder happens to spell the action's name, even though there is no
// "/{action}" segment after it. Require the action to be its own path
// segment — i.e. immediately preceded by "/" within the part of the path
// after the fixed prefix — so only a genuine "{symbol}/regimes" suffix
// dispatches to the regimes handler; a bare symbol always falls through to
// the detail handler regardless of what it's named.
func (r *SymbolRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	rest := strings.TrimPrefix(req.URL.Path, symbolDetailPrefix)
	if idx := strings.LastIndex(rest, "/"); idx >= 0 && rest[idx:] == mtfSuffix {
		r.regimes.ServeHTTP(w, req)
		return
	}
	r.detail.ServeHTTP(w, req)
}

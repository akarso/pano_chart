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
func (r *SymbolRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if strings.HasSuffix(req.URL.Path, mtfSuffix) {
		r.regimes.ServeHTTP(w, req)
		return
	}
	r.detail.ServeHTTP(w, req)
}

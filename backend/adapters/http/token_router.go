package http

import (
	"net/http"
	"strings"
)

// TokenRouter dispatches /api/token/{symbol}/{action} requests to the
// appropriate handler based on the action suffix.
type TokenRouter struct {
	setup     http.Handler
	fragility http.Handler
	behavior  http.Handler
}

// NewTokenRouter constructs a router for the /api/token/ prefix.
func NewTokenRouter(setup http.Handler, fragility http.Handler, behavior http.Handler) *TokenRouter {
	return &TokenRouter{setup: setup, fragility: fragility, behavior: behavior}
}

// ServeHTTP dispatches to the correct sub-handler.
func (r *TokenRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	switch {
	case strings.HasSuffix(path, "/setup"):
		r.setup.ServeHTTP(w, req)
	case strings.HasSuffix(path, "/fragility"):
		r.fragility.ServeHTTP(w, req)
	case strings.HasSuffix(path, "/behavior"):
		r.behavior.ServeHTTP(w, req)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown token endpoint")
	}
}

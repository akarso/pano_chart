package http

import (
	"net/http"
)

// RankingsRouter sets up the HTTP routes for rankings.
func RankingsRouter(handler http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/rankings", handler)
	return mux
}

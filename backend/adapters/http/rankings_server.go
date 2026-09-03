package http

import (
	"net/http"
)

// RankingsServer wires up the /api/rankings endpoint.
type RankingsServer struct {
	Handler http.Handler
}

func NewRankingsServer(handler http.Handler) *RankingsServer {
	return &RankingsServer{Handler: handler}
}

func (s *RankingsServer) Start(addr string) error {
	return http.ListenAndServe(addr, s.Handler)
}

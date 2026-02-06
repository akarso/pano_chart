package server

import (
	"fmt"
	"net/http"
	"time"

	adhttp "pano_chart/backend/adapters/http"
		"pano_chart/backend/adapters/infra"
		"pano_chart/backend/application/ports"
		"pano_chart/backend/application/usecases"
)

// Config holds composition inputs. Fields are minimal and injectable for tests.
type Config struct {
	Addr       string
	APIBaseURL string
	// Optional: if Repo is set, it will be used as the underlying repository instead of creating a FreeTier one.
	Repo ports.CandleRepositoryPort
	// Optional Redis client; if nil, no caching decorator is used.
	RedisClient infra.MinimalRedisClient
	CacheTTL   time.Duration
}

// NewApp wires the application components and returns an http.Handler that can be used by a server.
// It does not start any network listeners.
func NewApp(cfg Config) (http.Handler, error) {
	if cfg.Addr == "" {
		// default address if not provided
		cfg.Addr = ":8080"
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 5 * time.Minute
	}

	var repo ports.CandleRepositoryPort
	if cfg.Repo != nil {
		repo = cfg.Repo
	} else {
		if cfg.APIBaseURL == "" {
			return nil, fmt.Errorf("API base URL required when no Repo provided")
		}
		// create free-tier repository using default http client
		repo = infra.NewFreeTierCandleRepository(cfg.APIBaseURL, http.DefaultClient)
	}

	// Optionally wrap with Redis decorator
	if cfg.RedisClient != nil {
		repo = infra.NewRedisCandleRepository(cfg.RedisClient, repo, cfg.CacheTTL)
	}

	// Create use case
	uc := usecases.NewGetCandleSeries(repo)

	// Create HTTP handler
	h := adhttp.NewGetCandleSeriesHandler(uc)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/candles", h)

	return mux, nil
}

// StartServer is a convenience to start the HTTP server using the provided handler and address.
// This function blocks until the server returns an error.
func StartServer(handler http.Handler, addr string) error {
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
		// Keep other defaults minimal; graceful shutdown not in scope
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	return server.ListenAndServe()
}

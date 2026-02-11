package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/adapters/infra"
	"pano_chart/backend/application/usecases"
)

func main() {
	addr := ":8080"
	apiBase := os.Getenv("PC_API_BASE_URL")
	if apiBase == "" {
		apiBase = "https://api.coingecko.com/api/v3" // fallback for demo
	}

	// --- CandleRepository and SymbolUniverse wiring ---
	repo := infra.NewFreeTierCandleRepository(apiBase, nil)
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT"} // Add more as needed
	symUni, err := infra.NewStaticSymbolUniverse(symbols)
	if err != nil {
		log.Fatalf("failed to create symbol universe: %v", err)
	}
	syms, _ := symUni.ListSymbols()

	// --- Use cases ---
	getCandleUC := usecases.NewGetCandleSeries(repo)
	rankUC := usecases.NewDefaultRankSymbols(nil) // nil = default weights

	// --- Handlers ---
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Printf("/health write error: %v", err)
		}
	})
	mux.Handle("/api/v1/candles", adhttp.NewGetCandleSeriesHandler(getCandleUC))
	mux.Handle("/api/rankings", &adhttp.RankingsHandler{
		Ranker:     rankUC,
		CandleRepo: repo,
		Symbols:    syms,
	})

	fmt.Printf("Server starting on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

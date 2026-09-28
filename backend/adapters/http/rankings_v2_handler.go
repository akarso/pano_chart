package http

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"pano_chart/backend/application/usecases"
	"pano_chart/backend/domain"
)

const (
	// mtfOverlayConcurrency bounds in-flight ?mtf=1 store reads per request —
	// each row does up to 4 sequential GetSymbol calls (application/mtf), so
	// a naive per-row sequential loop could serialize hundreds of Redis
	// round-trips on the request goroutine.
	mtfOverlayConcurrency = 16
	// mtfOverlayBudget is a single deadline shared by every row's overlay
	// lookup, so a hung/slow store adds at most this much latency to the
	// response regardless of page size — not a per-row timeout multiplied
	// by however many rows are requested.
	mtfOverlayBudget = 300 * time.Millisecond
)

// RankingsV2Handler serves GET /api/rankings with sorting, pagination, and caching.
type RankingsV2Handler struct {
	useCase usecases.RankingsUseCase
	mtf     MTFCalculator // optional; nil disables the ?mtf=1 overlay
}

// NewRankingsV2Handler constructs the handler.
func NewRankingsV2Handler(uc usecases.RankingsUseCase) *RankingsV2Handler {
	return &RankingsV2Handler{useCase: uc}
}

// SetMTFCalculator wires the optional multi-timeframe alignment overlay
// (PR-099) exposed via the `?mtf=1` query param.
func (h *RankingsV2Handler) SetMTFCalculator(calc MTFCalculator) {
	h.mtf = calc
}

func (h *RankingsV2Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// --- Validate timeframe (required) ---
	tfStr := r.URL.Query().Get("timeframe")
	if tfStr == "" {
		writeRankingsError(w, "missing timeframe", http.StatusBadRequest)
		return
	}
	tf, err := domain.NewTimeframe(tfStr)
	if err != nil {
		writeRankingsError(w, "invalid timeframe", http.StatusBadRequest)
		return
	}

	// --- Parse sort (optional, default "total", unknown → "total") ---
	sortMode := usecases.ParseSortMode(r.URL.Query().Get("sort"))

	// --- Parse sidewaysAlgo (optional, empty = use default) ---
	sidewaysAlgo := usecases.ParseSidewaysAlgo(r.URL.Query().Get("sidewaysAlgo"))

	// --- Parse page (optional, default 1, clamp >=1) ---
	page := ParsePositiveIntOrDefault(r.URL.Query().Get("page"), 1)

	// --- Parse pageSize (optional, default 30, clamp 1–100) ---
	pageSize := ParsePositiveIntOrDefault(r.URL.Query().Get("pageSize"), 30)
	if pageSize > 200 {
		pageSize = 200
	}

	// --- Parse symbols (optional, comma-separated) ---
	var symbolFilter map[string]struct{}
	if raw := r.URL.Query().Get("symbols"); raw != "" {
		parts := strings.Split(raw, ",")
		symbolFilter = make(map[string]struct{}, len(parts))
		for _, s := range parts {
			s = strings.TrimSpace(s)
			if s != "" {
				symbolFilter[s] = struct{}{}
			}
		}
	}

	// --- Parse mtf (optional, default false) ---
	mtfRequested := r.URL.Query().Get("mtf") == "1"

	// --- Execute use case ---
	req := usecases.GetRankingsRequest{
		Timeframe:    tf,
		Sort:         sortMode,
		SidewaysAlgo: sidewaysAlgo,
	}
	out, err := h.useCase.Execute(r.Context(), req)
	if err != nil {
		writeRankingsError(w, "internal error", http.StatusInternalServerError)
		return
	}
	results := out.Results

	// --- Filter by symbols when requested ---
	if symbolFilter != nil {
		filtered := results[:0:0]
		for _, res := range results {
			if _, ok := symbolFilter[res.Symbol.String()]; ok {
				filtered = append(filtered, res)
			}
		}
		results = filtered
	}

	// --- Pagination ---
	totalItems := len(results)
	totalPages := 0
	if totalItems > 0 {
		totalPages = int(math.Ceil(float64(totalItems) / float64(pageSize)))
	}

	start := (page - 1) * pageSize
	if start > totalItems {
		start = totalItems
	}
	end := start + pageSize
	if end > totalItems {
		end = totalItems
	}
	pageSlice := results[start:end]

	// --- Build response ---
	respResults := make([]RankedResultV2Response, len(pageSlice))
	for i, row := range pageSlice {
		respResults[i] = RankedResultToV2(row)
	}
	if mtfRequested && h.mtf != nil {
		applyMTFOverlays(r.Context(), respResults, h.mtf, pageSlice)
	}

	precision := 0
	if len(results) > 0 && len(results[0].Sparkline) > 0 {
		precision = len(results[0].Sparkline)
	}

	resp := RankingsV2Response{
		Timeframe:     tfStr,
		Sort:          string(out.Sort),
		RequestedSort: string(out.RequestedSort),
		RSAvailable:   out.RSAvailable,
		Page:          page,
		PageSize:      pageSize,
		TotalItems:    totalItems,
		TotalPages:    totalPages,
		Precision:     precision,
		Results:       respResults,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		writeRankingsError(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// applyMTFOverlays fills each row's Alignment/AlignedState concurrently
// (mtfOverlayConcurrency in flight at once — application/mtf.Calculate is a
// handful of Redis round-trips, not free, and a plain per-row loop would
// serialize all of them on the request goroutine) under one shared deadline
// (mtfOverlayBudget), so a hung or slow store bounds the overlay's added
// latency instead of stalling the response indefinitely. resp and rows must
// be the same length and index-aligned; each goroutine only ever writes its
// own resp[i], so no synchronization is needed between them.
func applyMTFOverlays(
	ctx context.Context,
	resp []RankedResultV2Response,
	calc MTFCalculator,
	rows []usecases.RankedResult,
) {
	overlayCtx, cancel := context.WithTimeout(ctx, mtfOverlayBudget)
	defer cancel()

	var skipped int32
	var g errgroup.Group
	g.SetLimit(mtfOverlayConcurrency)
	for i := range resp {
		i := i
		symbol := rows[i].Symbol.String()
		g.Go(func() error {
			if !applyMTFOverlay(overlayCtx, &resp[i], calc, symbol) {
				atomic.AddInt32(&skipped, 1)
			}
			return nil
		})
	}
	_ = g.Wait() // applyMTFOverlay never returns an error to the group — each row is independent and best-effort.

	// One summary line, not one per row: a systemic outage would otherwise
	// flood logs with up to `len(resp)` near-simultaneous timeout lines.
	if skipped > 0 {
		log.Printf("[mtf] overlay incomplete: %d/%d rows skipped (store miss or timeout)", skipped, len(resp))
	}
}

// applyMTFOverlay fills resp's Alignment/AlignedState from calc for symbol.
// It is best-effort: any error (miss, store transport failure, or the
// shared overlay deadline expiring) leaves resp unchanged and reports false
// rather than failing the row — the overlay must never break the primary
// response (PR-099). An empty Stack (no fresh frames for this symbol —
// cold start, store outage) is not an error from Calculate's point of view,
// but it must still leave the fields unset: Alignment/AlignedState would
// otherwise read 0/"indecisive", which is indistinguishable from a real
// reading (COMMON.md says both are omitted when the store has nothing
// usable, not stamped with a fake zero value).
func applyMTFOverlay(ctx context.Context, resp *RankedResultV2Response, calc MTFCalculator, symbol string) bool {
	stack, err := calc.Calculate(ctx, symbol)
	if err != nil || len(stack.Frames) == 0 {
		return false
	}
	alignment := stack.Alignment
	alignedState := string(stack.AlignedState)
	resp.Alignment = &alignment
	resp.AlignedState = &alignedState
	return true
}

// ParsePositiveIntOrDefault parses a string to a positive int, returning def on failure or <=0.
func ParsePositiveIntOrDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return def
	}
	return v
}

func writeRankingsError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

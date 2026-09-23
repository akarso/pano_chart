package http

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	appsignal "pano_chart/backend/application/signal"
)

// ScorecardAPI is the application surface used by scorecard HTTP handlers.
type ScorecardAPI interface {
	Get(ctx context.Context, kind, label, tf, sinceRaw string) (appsignal.Scorecard, error)
	Summary(ctx context.Context, tf, sinceRaw string) (appsignal.SummaryResult, error)
}

// ScorecardHandler serves GET /api/scorecards and GET /api/scorecards/summary.
type ScorecardHandler struct {
	api ScorecardAPI
}

// NewScorecardHandler constructs the handler. api must be non-nil.
func NewScorecardHandler(api ScorecardAPI) *ScorecardHandler {
	return &ScorecardHandler{api: api}
}

// ServeHTTP routes on path suffix.
func (h *ScorecardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeScorecardErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h == nil || h.api == nil {
		writeScorecardErr(w, http.StatusServiceUnavailable, "scorecards unavailable")
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case strings.HasSuffix(path, "/scorecards/summary"):
		h.serveSummary(w, r)
	case strings.HasSuffix(path, "/scorecards"):
		h.serveGet(w, r)
	default:
		writeScorecardErr(w, http.StatusNotFound, "not found")
	}
}

func (h *ScorecardHandler) serveGet(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := strings.TrimSpace(q.Get("kind"))
	label := strings.TrimSpace(q.Get("label"))
	tf := strings.TrimSpace(q.Get("timeframe"))
	sinceRaw := strings.TrimSpace(q.Get("since"))
	if sinceRaw == "" {
		sinceRaw = "30d"
	}
	if kind == "" || label == "" {
		writeScorecardErr(w, http.StatusBadRequest, "kind and label are required")
		return
	}
	if _, err := appsignal.NormalizeKind(kind); err != nil {
		writeScorecardErr(w, http.StatusBadRequest, "unsupported kind")
		return
	}
	if _, err := appsignal.NormalizeTimeframe(tf); err != nil {
		writeScorecardErr(w, http.StatusBadRequest, "invalid timeframe")
		return
	}
	if strings.ContainsAny(label, "|\x00") {
		writeScorecardErr(w, http.StatusBadRequest, "invalid label")
		return
	}
	if _, err := appsignal.ResolveSince(sinceRaw, time.Now().UTC()); err != nil {
		writeScorecardErr(w, http.StatusBadRequest, "invalid since")
		return
	}
	card, err := h.api.Get(r.Context(), kind, label, tf, sinceRaw)
	if err != nil {
		writeScorecardAPIErr(w, err, "failed to build scorecard")
		return
	}
	writeScorecardJSON(w, card)
}

func (h *ScorecardHandler) serveSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tf := strings.TrimSpace(q.Get("timeframe"))
	sinceRaw := strings.TrimSpace(q.Get("since"))
	if sinceRaw == "" {
		sinceRaw = "30d"
	}
	if _, err := appsignal.NormalizeTimeframe(tf); err != nil {
		writeScorecardErr(w, http.StatusBadRequest, "invalid timeframe")
		return
	}
	if _, err := appsignal.ResolveSince(sinceRaw, time.Now().UTC()); err != nil {
		writeScorecardErr(w, http.StatusBadRequest, "invalid since")
		return
	}
	res, err := h.api.Summary(r.Context(), tf, sinceRaw)
	if err != nil {
		writeScorecardAPIErr(w, err, "failed to build summary")
		return
	}
	if res.Items == nil {
		res.Items = []appsignal.SummaryRow{}
	}
	writeScorecardJSON(w, res)
}

func writeScorecardAPIErr(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, context.Canceled):
		// Avoid net/http completing an unwritten response as 200.
		writeScorecardErr(w, statusClientClosedRequest, "client closed request")
	case errors.Is(err, appsignal.ErrValidation):
		writeScorecardErr(w, http.StatusBadRequest, "invalid request")
	case errors.Is(err, context.DeadlineExceeded):
		writeScorecardErr(w, http.StatusGatewayTimeout, "request deadline exceeded")
	default:
		writeScorecardErr(w, http.StatusBadGateway, fallback)
	}
}

// statusClientClosedRequest is the non-standard status nginx uses when the
// client disconnects (not 408 Request Timeout).
const statusClientClosedRequest = 499

func writeScorecardJSON(w http.ResponseWriter, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("[scorecard] marshal: %v", err)
		writeScorecardErr(w, http.StatusInternalServerError, "encode failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(b); err != nil {
		log.Printf("[scorecard] write: %v", err)
	}
	_, _ = w.Write([]byte("\n"))
}

func writeScorecardErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

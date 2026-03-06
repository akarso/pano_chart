package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"pano_chart/backend/application/usecases"
)

// NewsHandler handles GET /api/news and GET /api/news/{slug} requests.
type NewsHandler struct {
	newsUC usecases.NewsUseCase
}

// NewNewsHandler constructs the handler.
func NewNewsHandler(newsUC usecases.NewsUseCase) *NewsHandler {
	return &NewsHandler{newsUC: newsUC}
}

// newsListItemDTO for the list response.
type newsListItemDTO struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Date    string `json:"date"`
	Status  string `json:"status"`
	Excerpt string `json:"excerpt"`
	ETA     string `json:"eta,omitempty"`
}

// newsArticleDTO for the single article response.
type newsArticleDTO struct {
	Slug     string   `json:"slug"`
	Title    string   `json:"title"`
	Date     string   `json:"date"`
	Status   string   `json:"status"`
	Tags     []string `json:"tags"`
	Body     string   `json:"body"`
	ETA      string   `json:"eta,omitempty"`
	Priority string   `json:"priority,omitempty"`
}

// ServeHTTP implements http.Handler.
func (h *NewsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Path "/api/news" → list, "/api/news/{slug}" → single article
	slug := strings.TrimPrefix(r.URL.Path, "/api/news/")
	if r.URL.Path == "/api/news" || slug == "" {
		h.serveList(w, r)
		return
	}
	h.serveSingle(w, r, slug)
}

func (h *NewsHandler) serveList(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if lstr := r.URL.Query().Get("limit"); lstr != "" {
		l, err := strconv.Atoi(lstr)
		if err != nil || l <= 0 {
			http.Error(w, `{"error":"invalid limit"}`, http.StatusBadRequest)
			return
		}
		limit = l
	}

	articles, err := h.newsUC.List(r.Context(), limit)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	dtos := make([]newsListItemDTO, len(articles))
	for i, a := range articles {
		dto := newsListItemDTO{
			Slug:    a.Slug(),
			Title:   a.Title(),
			Date:    a.Date().Format("2006-01-02"),
			Status:  string(a.Status()),
			Excerpt: a.Excerpt(),
		}
		if a.ETA() != nil {
			dto.ETA = a.ETA().Format("2006-01-02")
		}
		dtos[i] = dto
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dtos); err != nil {
		http.Error(w, `{"error":"encode error"}`, http.StatusInternalServerError)
	}
}

func (h *NewsHandler) serveSingle(w http.ResponseWriter, r *http.Request, slug string) {
	article, err := h.newsUC.GetBySlug(r.Context(), slug)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	dto := newsArticleDTO{
		Slug:     article.Slug(),
		Title:    article.Title(),
		Date:     article.Date().Format("2006-01-02"),
		Status:   string(article.Status()),
		Tags:     article.Tags(),
		Body:     article.Body(),
		Priority: article.Priority(),
	}
	if article.ETA() != nil {
		dto.ETA = article.ETA().Format("2006-01-02")
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dto); err != nil {
		http.Error(w, `{"error":"encode error"}`, http.StatusInternalServerError)
	}
}

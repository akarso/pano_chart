package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	appsocial "pano_chart/backend/application/social"
)

// SocialFeedHandler handles GET /api/social/feed?handle=xxx.
type SocialFeedHandler struct {
	service *appsocial.Service
}

// NewSocialFeedHandler constructs the handler.
func NewSocialFeedHandler(service *appsocial.Service) *SocialFeedHandler {
	return &SocialFeedHandler{service: service}
}

type socialPostDTO struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	Author    string `json:"author"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Timestamp int64  `json:"timestamp"`
	IsRetweet bool   `json:"is_retweet"`
}

type socialFeedResponse struct {
	Handle string          `json:"handle"`
	Count  int             `json:"count"`
	Posts  []socialPostDTO `json:"posts"`
}

// ServeHTTP implements http.Handler.
func (h *SocialFeedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	handle := r.URL.Query().Get("handle")
	if handle == "" {
		http.Error(w, `{"error":"handle query parameter is required"}`, http.StatusBadRequest)
		return
	}

	// Build filter from optional query params.
	filter := appsocial.FeedFilter{}
	if r.URL.Query().Get("omit_retweets") == "true" {
		filter.OmitRetweets = true
	}
	if ml := r.URL.Query().Get("min_length"); ml != "" {
		if v, err := strconv.Atoi(ml); err == nil && v > 0 {
			filter.MinLength = v
		}
	}
	if kw := r.URL.Query().Get("keywords"); kw != "" {
		parts := strings.Split(kw, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				filter.Keywords = append(filter.Keywords, p)
			}
		}
	}

	posts, err := h.service.FilteredFeed(handle, filter)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	dtos := make([]socialPostDTO, len(posts))
	for i, p := range posts {
		dtos[i] = socialPostDTO{
			ID:        p.ID,
			AccountID: p.AccountID,
			Author:    p.Author,
			Title:     p.Title,
			URL:       p.URL,
			Timestamp: p.Timestamp,
			IsRetweet: p.IsRetweet,
		}
	}

	resp := socialFeedResponse{
		Handle: handle,
		Count:  len(dtos),
		Posts:  dtos,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, `{"error":"failed to encode response"}`, http.StatusInternalServerError)
		return
	}
}

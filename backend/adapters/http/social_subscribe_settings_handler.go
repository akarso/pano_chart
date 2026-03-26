package http

import (
	"encoding/json"
	"net/http"
	"strings"

	appsocial "pano_chart/backend/application/social"
)

// SocialSubscribeSettingsHandler handles PUT /api/social/subscribe/settings.
type SocialSubscribeSettingsHandler struct {
	service *appsocial.Service
}

// NewSocialSubscribeSettingsHandler constructs the handler.
func NewSocialSubscribeSettingsHandler(service *appsocial.Service) *SocialSubscribeSettingsHandler {
	return &SocialSubscribeSettingsHandler{service: service}
}

type subscribeSettingsRequest struct {
	UserID       string   `json:"user_id"`
	Handle       string   `json:"handle"`
	OmitRetweets bool     `json:"omit_retweets"`
	OmitReplies  bool     `json:"omit_replies"`
	MinLength    int      `json:"min_length"`
	Keywords     []string `json:"keywords"`
}

// ServeHTTP implements http.Handler.
func (h *SocialSubscribeSettingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req subscribeSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.UserID == "" || req.Handle == "" {
		http.Error(w, `{"error":"user_id and handle are required"}`, http.StatusBadRequest)
		return
	}

	// Sanitize keywords.
	var keywords []string
	for _, kw := range req.Keywords {
		kw = strings.TrimSpace(kw)
		if kw != "" {
			keywords = append(keywords, kw)
		}
	}

	filter := appsocial.FeedFilter{
		OmitRetweets: req.OmitRetweets,
		OmitReplies:  req.OmitReplies,
		MinLength:    req.MinLength,
		Keywords:     keywords,
	}

	if err := h.service.SetFilterConfig(req.UserID, req.Handle, filter); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

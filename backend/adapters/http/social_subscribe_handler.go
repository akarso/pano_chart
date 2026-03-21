package http

import (
	"encoding/json"
	"net/http"

	appsocial "pano_chart/backend/application/social"
)

// SocialSubscribeHandler handles POST /api/social/subscribe.
type SocialSubscribeHandler struct {
	service *appsocial.Service
}

// NewSocialSubscribeHandler constructs the handler.
func NewSocialSubscribeHandler(service *appsocial.Service) *SocialSubscribeHandler {
	return &SocialSubscribeHandler{service: service}
}

type socialSubscribeRequest struct {
	UserID string `json:"user_id"`
	Handle string `json:"handle"`
}

// ServeHTTP implements http.Handler.
func (h *SocialSubscribeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req socialSubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.UserID == "" || req.Handle == "" {
		http.Error(w, `{"error":"user_id and handle are required"}`, http.StatusBadRequest)
		return
	}

	if err := h.service.Subscribe(req.UserID, req.Handle); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "subscribed"})
}

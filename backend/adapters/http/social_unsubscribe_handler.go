package http

import (
	"encoding/json"
	"net/http"

	appsocial "pano_chart/backend/application/social"
)

// SocialUnsubscribeHandler handles POST /api/social/unsubscribe.
type SocialUnsubscribeHandler struct {
	service *appsocial.Service
}

// NewSocialUnsubscribeHandler constructs the handler.
func NewSocialUnsubscribeHandler(service *appsocial.Service) *SocialUnsubscribeHandler {
	return &SocialUnsubscribeHandler{service: service}
}

type socialUnsubscribeRequest struct {
	UserID string `json:"user_id"`
	Handle string `json:"handle"`
}

// ServeHTTP implements http.Handler.
func (h *SocialUnsubscribeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req socialUnsubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.UserID == "" || req.Handle == "" {
		http.Error(w, `{"error":"user_id and handle are required"}`, http.StatusBadRequest)
		return
	}

	if err := h.service.Unsubscribe(req.UserID, req.Handle); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "unsubscribed"})
}

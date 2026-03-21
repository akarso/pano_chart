package http

import (
	"encoding/json"
	"net/http"

	appsocial "pano_chart/backend/application/social"
)

// SocialAccountsHandler handles GET /api/social/accounts?user_id=xxx.
type SocialAccountsHandler struct {
	service *appsocial.Service
}

// NewSocialAccountsHandler constructs the handler.
func NewSocialAccountsHandler(service *appsocial.Service) *SocialAccountsHandler {
	return &SocialAccountsHandler{service: service}
}

type socialAccountsResponse struct {
	UserID   string   `json:"user_id"`
	Count    int      `json:"count"`
	Accounts []string `json:"accounts"`
}

// ServeHTTP implements http.Handler.
func (h *SocialAccountsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, `{"error":"user_id query parameter is required"}`, http.StatusBadRequest)
		return
	}

	accounts, err := h.service.AccountsForUser(userID)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	resp := socialAccountsResponse{
		UserID:   userID,
		Count:    len(accounts),
		Accounts: accounts,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, `{"error":"failed to encode response"}`, http.StatusInternalServerError)
		return
	}
}

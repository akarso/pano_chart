package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"pano_chart/backend/adapters/http/middleware"
	appsocial "pano_chart/backend/application/social"
)

// DeviceRegisterHandler handles POST /api/device/register.
type DeviceRegisterHandler struct {
	devices appsocial.DeviceTokenStore
}

// NewDeviceRegisterHandler constructs the handler.
func NewDeviceRegisterHandler(devices appsocial.DeviceTokenStore) *DeviceRegisterHandler {
	return &DeviceRegisterHandler{devices: devices}
}

type deviceRegisterRequest struct {
	// UserID is accepted for backward-compatible JSON decoding but is no
	// longer trusted — the authenticated user ID from the request context
	// is used instead. Kept only so older client payloads don't fail to
	// decode; safe to remove once no client sends it anymore.
	UserID   string `json:"user_id,omitempty"`
	DeviceID string `json:"device_id"`
	FCMToken string `json:"fcm_token"`
	Platform string `json:"platform"`
}

// ServeHTTP implements http.Handler. Must be registered behind
// middleware.RequireAuth.
func (h *DeviceRegisterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req deviceRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" || req.FCMToken == "" || req.Platform == "" {
		http.Error(w, `{"error":"device_id, fcm_token, and platform are required"}`, http.StatusBadRequest)
		return
	}

	if req.Platform != "android" && req.Platform != "ios" {
		http.Error(w, `{"error":"platform must be android or ios"}`, http.StatusBadRequest)
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	if err := h.devices.Register(userID, req.DeviceID, req.FCMToken, req.Platform); err != nil {
		if errors.Is(err, appsocial.ErrDeviceOwnedByAnotherUser) {
			http.Error(w, `{"error":"device_id already registered to a different account"}`, http.StatusConflict)
			return
		}
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

// DeviceUnregisterHandler handles POST /api/device/unregister.
type DeviceUnregisterHandler struct {
	devices appsocial.DeviceTokenStore
}

// NewDeviceUnregisterHandler constructs the handler.
func NewDeviceUnregisterHandler(devices appsocial.DeviceTokenStore) *DeviceUnregisterHandler {
	return &DeviceUnregisterHandler{devices: devices}
}

type deviceUnregisterRequest struct {
	DeviceID string `json:"device_id"`
}

// ServeHTTP implements http.Handler. Must be registered behind
// middleware.RequireAuth. Unregister is scoped to the authenticated user so
// one user cannot silence another's push notifications by guessing/reusing
// a device ID.
func (h *DeviceUnregisterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req deviceUnregisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" {
		http.Error(w, `{"error":"device_id is required"}`, http.StatusBadRequest)
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	if err := h.devices.Unregister(userID, req.DeviceID); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "unregistered"})
}

package http

import (
	"encoding/json"
	"net/http"

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
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
	FCMToken string `json:"fcm_token"`
	Platform string `json:"platform"`
}

// ServeHTTP implements http.Handler.
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

	if req.UserID == "" || req.DeviceID == "" || req.FCMToken == "" || req.Platform == "" {
		http.Error(w, `{"error":"user_id, device_id, fcm_token, and platform are required"}`, http.StatusBadRequest)
		return
	}

	if req.Platform != "android" && req.Platform != "ios" {
		http.Error(w, `{"error":"platform must be android or ios"}`, http.StatusBadRequest)
		return
	}

	if err := h.devices.Register(req.UserID, req.DeviceID, req.FCMToken, req.Platform); err != nil {
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

// ServeHTTP implements http.Handler.
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

	if err := h.devices.Unregister(req.DeviceID); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "unregistered"})
}

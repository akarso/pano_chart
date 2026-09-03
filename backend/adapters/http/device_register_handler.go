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
	// UserID is the authenticated user ID whenever RequireAuth verifies a
	// secret. It's only read from the request body as a migration-window
	// fallback for a pre-PR-070 client that has no secret yet (see
	// NewDeviceRegisterHandler's doc) — once RequireAuth enforces, this
	// field is never consulted and can be removed.
	UserID   string `json:"user_id"`
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

	userID, ok := middleware.UserIDOrLegacyFallback(r.Context(), req.UserID)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
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
//
// Unlike Register/notification-config/subscription-status, this endpoint
// has no migration-window fallback: the pre-PR-070 version of this route
// never took a client-supplied identity at all (it unregistered by
// device_id alone, with no ownership check — that was the original IDOR).
// So there's no legacy field to trust here even temporarily; an
// unauthenticated caller is simply rejected regardless of RequireAuth's
// enforce setting. In practice this is moot today — no client build calls
// this endpoint.
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

	userID, ok := middleware.UserIDFromContextOK(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if err := h.devices.Unregister(userID, req.DeviceID); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "unregistered"})
}

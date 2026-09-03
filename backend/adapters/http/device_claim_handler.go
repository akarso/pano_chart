package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"pano_chart/backend/application/usecases"
)

// maxClaimBodyBytes caps the request body for the (unauthenticated, public)
// claim endpoint — the payload is at most one short field, anything larger
// is abuse.
const maxClaimBodyBytes = 1024

type deviceClaimRequest struct {
	// ExistingUserID binds the new secret to a pre-existing locally
	// generated ID (pre-PR-070 clients) instead of minting a fresh one.
	ExistingUserID string `json:"existingUserId,omitempty"`
}

// NewDeviceClaimHandler returns an http.HandlerFunc that issues a new
// server-signed device credential.
//
//	POST /api/device/claim
//	Body: { "existingUserId": "..." }  // optional
//	Resp: { "userId": "...", "secret": "..." }
//
// This endpoint is intentionally NOT behind middleware.RequireAuth — it's
// how a client obtains credentials in the first place.
func NewDeviceClaimHandler(uc usecases.ClaimDevice) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxClaimBodyBytes)

		var req deviceClaimRequest
		if r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
				return
			}
		}

		result, err := uc.Execute(r.Context(), usecases.ClaimDeviceInput{
			ExistingUserID: req.ExistingUserID,
		})
		if err != nil {
			switch {
			case errors.Is(err, usecases.ErrInvalidUserID):
				http.Error(w, `{"error":"invalid existingUserId"}`, http.StatusBadRequest)
			case errors.Is(err, usecases.ErrUserIDAlreadyClaimed):
				http.Error(w, `{"error":"user id already claimed"}`, http.StatusConflict)
			default:
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"userId": result.UserID,
			"secret": result.Secret,
		})
	}
}

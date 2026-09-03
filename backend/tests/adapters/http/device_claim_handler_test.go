package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/usecases"
)

type fakeClaimDevice struct {
	lastInput usecases.ClaimDeviceInput
	result    usecases.ClaimDeviceResult
	err       error
}

func (f *fakeClaimDevice) Execute(_ context.Context, input usecases.ClaimDeviceInput) (usecases.ClaimDeviceResult, error) {
	f.lastInput = input
	if f.err != nil {
		return usecases.ClaimDeviceResult{}, f.err
	}
	return f.result, nil
}

func TestDeviceClaimHandler_NewDevice(t *testing.T) {
	uc := &fakeClaimDevice{result: usecases.ClaimDeviceResult{UserID: "user1", Secret: "sekret"}}
	handler := adhttp.NewDeviceClaimHandler(uc)

	req := httptest.NewRequest(http.MethodPost, "/api/device/claim", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "user1", body["userId"])
	assert.Equal(t, "sekret", body["secret"])
	assert.Empty(t, uc.lastInput.ExistingUserID)
}

func TestDeviceClaimHandler_ExistingUserID_PassedThrough(t *testing.T) {
	uc := &fakeClaimDevice{result: usecases.ClaimDeviceResult{UserID: "legacy-1", Secret: "sekret"}}
	handler := adhttp.NewDeviceClaimHandler(uc)

	body, _ := json.Marshal(map[string]string{"existingUserId": "legacy-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/device/claim", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "legacy-1", uc.lastInput.ExistingUserID)
}

func TestDeviceClaimHandler_MethodNotAllowed(t *testing.T) {
	uc := &fakeClaimDevice{}
	handler := adhttp.NewDeviceClaimHandler(uc)

	req := httptest.NewRequest(http.MethodGet, "/api/device/claim", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Result().StatusCode)
}

func TestDeviceClaimHandler_InvalidBody(t *testing.T) {
	uc := &fakeClaimDevice{}
	handler := adhttp.NewDeviceClaimHandler(uc)

	req := httptest.NewRequest(http.MethodPost, "/api/device/claim", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestDeviceClaimHandler_UseCaseError(t *testing.T) {
	uc := &fakeClaimDevice{err: assert.AnError}
	handler := adhttp.NewDeviceClaimHandler(uc)

	req := httptest.NewRequest(http.MethodPost, "/api/device/claim", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
}

func TestDeviceClaimHandler_AlreadyClaimed_409(t *testing.T) {
	uc := &fakeClaimDevice{err: usecases.ErrUserIDAlreadyClaimed}
	handler := adhttp.NewDeviceClaimHandler(uc)

	req := httptest.NewRequest(http.MethodPost, "/api/device/claim", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Result().StatusCode)
}

func TestDeviceClaimHandler_InvalidUserID_400(t *testing.T) {
	uc := &fakeClaimDevice{err: usecases.ErrInvalidUserID}
	handler := adhttp.NewDeviceClaimHandler(uc)

	req := httptest.NewRequest(http.MethodPost, "/api/device/claim", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestDeviceClaimHandler_OversizedBody_Rejected(t *testing.T) {
	uc := &fakeClaimDevice{result: usecases.ClaimDeviceResult{UserID: "u1", Secret: "s"}}
	handler := adhttp.NewDeviceClaimHandler(uc)

	huge := strings.Repeat("a", 2000)
	body, _ := json.Marshal(map[string]string{"existingUserId": huge})
	req := httptest.NewRequest(http.MethodPost, "/api/device/claim", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// ContentLength must reflect the oversized body for MaxBytesReader to
	// kick in via the decoder hitting the limit mid-stream.
	req.ContentLength = int64(len(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

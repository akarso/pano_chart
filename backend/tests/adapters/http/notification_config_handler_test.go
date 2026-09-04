package http_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/adapters/http/middleware"
	appnotify "pano_chart/backend/application/notifications"
)

type fakeNotificationConfigStore struct {
	saved   appnotify.NotificationConfig
	get     appnotify.NotificationConfig
	getErr  error
	saveErr error
}

func (f *fakeNotificationConfigStore) Get(userID string) (appnotify.NotificationConfig, error) {
	if f.getErr != nil {
		return appnotify.NotificationConfig{}, f.getErr
	}
	f.get.UserID = userID
	return f.get, nil
}

func (f *fakeNotificationConfigStore) Save(cfg appnotify.NotificationConfig) error {
	f.saved = cfg
	return f.saveErr
}

func (f *fakeNotificationConfigStore) All() ([]appnotify.NotificationConfig, error) {
	return []appnotify.NotificationConfig{f.saved}, nil
}

func TestNotificationConfigHandler_Get_UsesAuthenticatedUserID(t *testing.T) {
	store := &fakeNotificationConfigStore{}
	handler := adhttp.NewNotificationConfigHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/notification/config", nil)
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	var resp map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, "user1", resp["user_id"])
}

func TestNotificationConfigHandler_Get_NoAuthContext_NoLegacyParam_401(t *testing.T) {
	store := &fakeNotificationConfigStore{}
	handler := adhttp.NewNotificationConfigHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/notification/config", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)
}

func TestNotificationConfigHandler_Get_LogOnlyMigrationFallback_UsesLegacyQueryParam(t *testing.T) {
	store := &fakeNotificationConfigStore{}
	handler := adhttp.NewNotificationConfigHandler(store)

	// No auth context (as RequireAuth would leave it in log-only mode for
	// a pre-PR-070 client), but the client still sends the old ?user_id=.
	req := httptest.NewRequest(http.MethodGet, "/api/notification/config?user_id=legacy-user", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	var resp map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, "legacy-user", resp["user_id"])
}

func TestNotificationConfigHandler_Put_NoAuthContext_NoLegacyBody_401(t *testing.T) {
	store := &fakeNotificationConfigStore{}
	handler := adhttp.NewNotificationConfigHandler(store)

	body, _ := json.Marshal(map[string]interface{}{"uptrend": true})
	req := httptest.NewRequest(http.MethodPut, "/api/notification/config", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)
}

func TestNotificationConfigHandler_Put_LogOnlyMigrationFallback_UsesLegacyBodyUserID(t *testing.T) {
	store := &fakeNotificationConfigStore{}
	handler := adhttp.NewNotificationConfigHandler(store)

	// No auth context, but the pre-PR-070 client still sends user_id in
	// the body — must be trusted only because there's no verified identity
	// available yet (log-only migration window).
	body, _ := json.Marshal(map[string]interface{}{
		"user_id": "legacy-user",
		"uptrend": true,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/notification/config", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "legacy-user", store.saved.UserID)
}

func TestNotificationConfigHandler_Put_IgnoresClientSuppliedUserID(t *testing.T) {
	store := &fakeNotificationConfigStore{}
	handler := adhttp.NewNotificationConfigHandler(store)

	// Body claims to be "attacker", but the authenticated context says "victim".
	body, _ := json.Marshal(map[string]interface{}{
		"user_id": "attacker",
		"uptrend": true,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/notification/config", bytes.NewReader(body))
	req = req.WithContext(middleware.WithUserID(req.Context(), "victim"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "victim", store.saved.UserID)
}

func TestNotificationConfigHandler_Put_InvalidBody(t *testing.T) {
	store := &fakeNotificationConfigStore{}
	handler := adhttp.NewNotificationConfigHandler(store)

	req := httptest.NewRequest(http.MethodPut, "/api/notification/config", bytes.NewReader([]byte("not json")))
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestNotificationConfigHandler_Get_StoreError(t *testing.T) {
	store := &fakeNotificationConfigStore{getErr: fmt.Errorf("db down")}
	handler := adhttp.NewNotificationConfigHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/notification/config", nil)
	req = req.WithContext(middleware.WithUserID(req.Context(), "user1"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode)
}

func TestNotificationConfigHandler_MethodNotAllowed(t *testing.T) {
	store := &fakeNotificationConfigStore{}
	handler := adhttp.NewNotificationConfigHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/notification/config", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Result().StatusCode)
}

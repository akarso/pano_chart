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

func TestNotificationConfigHandler_Get_NoAuthContext_Panics(t *testing.T) {
	store := &fakeNotificationConfigStore{}
	handler := adhttp.NewNotificationConfigHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/notification/config", nil)
	w := httptest.NewRecorder()

	assert.Panics(t, func() { handler.ServeHTTP(w, req) })
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

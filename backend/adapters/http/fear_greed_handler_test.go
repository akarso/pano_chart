package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	adhttp "pano_chart/backend/adapters/http"
	"pano_chart/backend/application/usecases"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeFearGreedUC struct {
	result *usecases.FearGreedResult
	err    error
}

func (f *fakeFearGreedUC) Execute(_ context.Context) (*usecases.FearGreedResult, error) {
	return f.result, f.err
}

func TestFearGreedHandler_Success(t *testing.T) {
	uc := &fakeFearGreedUC{result: &usecases.FearGreedResult{
		Value:               14,
		ValueClassification: "Extreme Fear",
		Timestamp:           1772496000,
	}}
	handler := adhttp.NewFearGreedHandler(uc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/fear-greed", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(14), resp["value"])
	assert.Equal(t, "Extreme Fear", resp["valueClassification"])
	assert.NotEmpty(t, resp["timestampUtc"])
}

func TestFearGreedHandler_MethodNotAllowed(t *testing.T) {
	handler := adhttp.NewFearGreedHandler(&fakeFearGreedUC{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/fear-greed", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestFearGreedHandler_UseCaseError(t *testing.T) {
	uc := &fakeFearGreedUC{err: errors.New("upstream fail")}
	handler := adhttp.NewFearGreedHandler(uc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/fear-greed", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

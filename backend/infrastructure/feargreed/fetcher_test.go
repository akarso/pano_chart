package feargreed_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pano_chart/backend/infrastructure/feargreed"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetcher_Execute_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"name": "Fear and Greed Index",
			"data": []map[string]string{
				{
					"value":                "14",
					"value_classification": "Extreme Fear",
					"timestamp":            "1772496000",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	f := feargreed.NewFetcher(srv.Client(), srv.URL)
	result, err := f.Execute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 14, result.Value)
	assert.Equal(t, "Extreme Fear", result.ValueClassification)
	assert.Equal(t, int64(1772496000), result.Timestamp)
}

func TestFetcher_Execute_EmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": []map[string]string{},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	f := feargreed.NewFetcher(srv.Client(), srv.URL)
	_, err := f.Execute(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty data")
}

func TestFetcher_Execute_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := feargreed.NewFetcher(srv.Client(), srv.URL)
	_, err := f.Execute(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

func TestFetcher_Execute_InvalidValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": []map[string]string{
				{
					"value":                "abc",
					"value_classification": "Fear",
					"timestamp":            "1772496000",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	f := feargreed.NewFetcher(srv.Client(), srv.URL)
	_, err := f.Execute(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse value")
}

func TestFetcher_DefaultsToStandardURL(t *testing.T) {
	f := feargreed.NewFetcher(nil, "")
	// Just ensure it constructs without panic — we can't test the real URL here
	assert.NotNil(t, f)
}

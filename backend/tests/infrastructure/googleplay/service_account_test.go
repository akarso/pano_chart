package googleplay_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"pano_chart/backend/infrastructure/googleplay"
)

func TestNewServiceAccountClient_FileNotFound(t *testing.T) {
	_, err := googleplay.NewServiceAccountClient("/nonexistent/service-account.json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading service account JSON")
}

func TestNewServiceAccountClient_InvalidJSON(t *testing.T) {
	tmpFile := t.TempDir() + "/bad.json"
	if err := os.WriteFile(tmpFile, []byte("not-json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := googleplay.NewServiceAccountClient(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing service account credentials")
}

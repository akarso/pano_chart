package googleplay

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2/google"
)

// androidPublisherScope is the OAuth2 scope required to call the
// Google Play Developer API.
const androidPublisherScope = "https://www.googleapis.com/auth/androidpublisher"

// NewServiceAccountClient creates an *http.Client that automatically
// obtains and refreshes OAuth2 access tokens using a Google service
// account JSON key file.
//
// The returned client can be passed directly to [NewProvider]; there is
// no need to set [Config.AccessToken] because the transport injects
// the Bearer token on every request.
func NewServiceAccountClient(jsonPath string) (*http.Client, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("reading service account JSON %s: %w", jsonPath, err)
	}

	cfg, err := google.JWTConfigFromJSON(data, androidPublisherScope)
	if err != nil {
		return nil, fmt.Errorf("parsing service account credentials: %w", err)
	}

	return cfg.Client(context.Background()), nil
}

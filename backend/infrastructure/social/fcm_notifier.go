package social

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	appsocial "pano_chart/backend/application/social"
)

// Compile-time check.
var _ appsocial.PushNotifier = (*FCMNotifier)(nil)

// FCMNotifier sends push notifications via the FCM HTTP v1 API using a
// Google service account for authentication.
type FCMNotifier struct {
	projectID  string
	serviceKey *serviceAccountKey
	client     *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// serviceAccountKey holds the fields we need from the service account JSON.
type serviceAccountKey struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
	privateKey  *rsa.PrivateKey
}

// NewFCMNotifier creates a notifier from the service account JSON file
// pointed to by credentialsPath. If projectID is empty, it is read from
// the JSON file.
func NewFCMNotifier(credentialsPath, projectID string) (*FCMNotifier, error) {
	data, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("reading credentials: %w", err)
	}

	var key serviceAccountKey
	if err := json.Unmarshal(data, &key); err != nil {
		return nil, fmt.Errorf("parsing credentials JSON: %w", err)
	}

	block, _ := pem.Decode([]byte(key.PrivateKey))
	if block == nil {
		return nil, fmt.Errorf("no PEM block in private_key")
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}
	key.privateKey = rsaKey

	if key.TokenURI == "" {
		key.TokenURI = "https://oauth2.googleapis.com/token"
	}

	if projectID == "" {
		projectID = key.ProjectID
	}
	if projectID == "" {
		return nil, fmt.Errorf("project_id not found in credentials and not provided")
	}

	return &FCMNotifier{
		projectID:  projectID,
		serviceKey: &key,
		client:     &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// Send delivers a push notification to a single FCM token.
func (n *FCMNotifier) Send(ctx context.Context, token, title, body string) error {
	accessToken, err := n.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("getting access token: %w", err)
	}

	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", n.projectID)

	payload := fcmRequest{
		Message: fcmMessage{
			Token: token,
			Notification: &fcmNotification{
				Title: title,
				Body:  body,
			},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending FCM request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("FCM returned %d: %s", resp.StatusCode, string(respBody))
}

// getAccessToken returns a cached or freshly minted OAuth2 access token.
func (n *FCMNotifier) getAccessToken(ctx context.Context) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Return cached token if still valid (with 5 min margin).
	if n.accessToken != "" && time.Now().Before(n.tokenExpiry.Add(-5*time.Minute)) {
		return n.accessToken, nil
	}

	tok, expiry, err := n.mintAccessToken(ctx)
	if err != nil {
		return "", err
	}
	n.accessToken = tok
	n.tokenExpiry = expiry
	log.Printf("[fcm] refreshed access token, expires %s", expiry.Format(time.RFC3339))
	return tok, nil
}

// mintAccessToken creates a JWT, exchanges it at Google's token endpoint,
// and returns the access token + expiry.
func (n *FCMNotifier) mintAccessToken(ctx context.Context) (string, time.Time, error) {
	now := time.Now()

	jwt, err := signJWT(n.serviceKey, now)
	if err != nil {
		return "", time.Time{}, err
	}

	// Exchange JWT for access token.
	form := "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&assertion=" + jwt
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.serviceKey.TokenURI,
		bytes.NewBufferString(form))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := n.client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("token exchange %d: %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", time.Time{}, fmt.Errorf("decoding token response: %w", err)
	}

	expiry := now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return tokenResp.AccessToken, expiry, nil
}

// ── FCM payload types ───────────────────────────────────────────────────────

type fcmRequest struct {
	Message fcmMessage `json:"message"`
}

type fcmMessage struct {
	Token        string           `json:"token"`
	Notification *fcmNotification `json:"notification,omitempty"`
}

type fcmNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

package social

import (
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// signJWT creates a signed JWT assertion for the Google OAuth2 token
// endpoint, requesting the FCM scope. Uses RS256 (RSASSA-PKCS1-v1_5 + SHA-256).
func signJWT(key *serviceAccountKey, now time.Time) (string, error) {
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}
	headerJSON, _ := json.Marshal(header)

	claims := map[string]any{
		"iss":   key.ClientEmail,
		"scope": "https://www.googleapis.com/auth/firebase.messaging",
		"aud":   key.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(1 * time.Hour).Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := headerB64 + "." + claimsB64

	hash := sha256.Sum256([]byte(signingInput))
	sig, err := key.privateKey.Sign(rand.Reader, hash[:], crypto.SHA256)
	if err != nil {
		return "", fmt.Errorf("signing JWT: %w", err)
	}

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return signingInput + "." + sigB64, nil
}

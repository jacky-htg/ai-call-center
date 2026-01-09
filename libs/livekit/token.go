package livekit

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

// GenerateAccessToken creates a LiveKit-compatible access token using HMAC-SHA256.
// This is a lightweight implementation that produces a JWT with a 'video' grant
// containing the room name. It uses apiKey as the 'iss' claim and signs with apiSecret.
func GenerateAccessToken(apiKey, apiSecret, room, identity string, ttlSeconds int) (string, error) {
	if apiKey == "" || apiSecret == "" {
		return "", fmt.Errorf("livekit api key/secret required")
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}

	now := time.Now().Unix()
	exp := time.Now().Add(time.Duration(ttlSeconds) * time.Second).Unix()

	// random jti
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate jti: %w", err)
	}
	jti := hex.EncodeToString(b)

	claims := jwt.MapClaims{
		"jti":   jti,
		"iss":   apiKey,
		"nbf":   now,
		"exp":   exp,
		"sub":   "",
		"name":  identity,
		"video": map[string]interface{}{"room": room},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// sign with apiSecret
	signed, err := token.SignedString([]byte(apiSecret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

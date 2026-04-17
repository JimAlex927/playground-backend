package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	domainauth "playground/internal/domain/auth"
)

type TokenManager struct {
	secret []byte
}

func NewTokenManager(secret string) TokenManager {
	return TokenManager{
		secret: []byte(secret),
	}
}

func (m TokenManager) Issue(baseClaims domainauth.Claims, ttl time.Duration) (string, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	claims := baseClaims
	claims.IssuedAt = now
	claims.ExpiresAt = expiresAt

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal token header: %w", err)
	}

	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal token payload: %w", err)
	}

	unsigned := encodeSegment(headerJSON) + "." + encodeSegment(payloadJSON)
	signature := m.sign(unsigned)
	return unsigned + "." + signature, expiresAt, nil
}

func (m TokenManager) Parse(token string) (domainauth.Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return domainauth.Claims{}, fmt.Errorf("token format is invalid")
	}

	unsigned := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(m.sign(unsigned))) {
		return domainauth.Claims{}, fmt.Errorf("token signature is invalid")
	}

	payload, err := decodeSegment(parts[1])
	if err != nil {
		return domainauth.Claims{}, fmt.Errorf("decode token payload: %w", err)
	}

	var claims domainauth.Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return domainauth.Claims{}, fmt.Errorf("unmarshal token payload: %w", err)
	}

	if time.Now().UTC().After(claims.ExpiresAt) {
		return domainauth.Claims{}, fmt.Errorf("token expired")
	}

	return claims, nil
}

func (m TokenManager) sign(value string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(value))
	return encodeSegment(mac.Sum(nil))
}

func encodeSegment(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func decodeSegment(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(value)
}

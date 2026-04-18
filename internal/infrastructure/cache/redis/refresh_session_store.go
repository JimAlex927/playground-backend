package rediscache

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"playground/internal/config"
	domainauth "playground/internal/domain/auth"
)

type RefreshSessionStore struct {
	client *redis.Client
	prefix string
}

func NewRefreshSessionStore(client *redis.Client, cfg config.RedisConfig) *RefreshSessionStore {
	if client == nil {
		return nil
	}
	return &RefreshSessionStore{
		client: client,
		prefix: normalizePrefix(cfg.KeyPrefix),
	}
}

func (s *RefreshSessionStore) Create(ctx context.Context, session domainauth.RefreshSession, ttl time.Duration) (string, time.Time, error) {
	if s == nil || s.client == nil {
		return "", time.Time{}, errors.New("refresh session store is unavailable")
	}

	token, err := newOpaqueToken()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate refresh token: %w", err)
	}

	payload, err := json.Marshal(session)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("encode refresh session: %w", err)
	}

	expiresAt := time.Now().UTC().Add(ttl)
	if err := s.client.Set(ctx, s.sessionKey(token), payload, ttl).Err(); err != nil {
		return "", time.Time{}, fmt.Errorf("persist refresh session: %w", err)
	}
	return token, expiresAt, nil
}

func (s *RefreshSessionStore) Get(ctx context.Context, token string) (domainauth.RefreshSession, bool, error) {
	if s == nil || s.client == nil {
		return domainauth.RefreshSession{}, false, nil
	}

	value, err := s.client.Get(ctx, s.sessionKey(token)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return domainauth.RefreshSession{}, false, nil
		}
		return domainauth.RefreshSession{}, false, fmt.Errorf("get refresh session: %w", err)
	}

	var session domainauth.RefreshSession
	if err := json.Unmarshal(value, &session); err != nil {
		return domainauth.RefreshSession{}, false, fmt.Errorf("decode refresh session: %w", err)
	}
	return session, true, nil
}

func (s *RefreshSessionStore) Delete(ctx context.Context, token string) error {
	if s == nil || s.client == nil || strings.TrimSpace(token) == "" {
		return nil
	}
	if err := s.client.Del(ctx, s.sessionKey(token)).Err(); err != nil {
		return fmt.Errorf("delete refresh session: %w", err)
	}
	return nil
}

func (s *RefreshSessionStore) sessionKey(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return fmt.Sprintf("%s:auth:refresh:%s", s.prefix, hex.EncodeToString(sum[:]))
}

func newOpaqueToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

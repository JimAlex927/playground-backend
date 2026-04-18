package rediscache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"playground/internal/config"
	domainauth "playground/internal/domain/auth"
)

type PrincipalCache struct {
	client *redis.Client
	prefix string
	ttl    time.Duration
}

func NewClient(cfg config.RedisConfig) (*redis.Client, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:     strings.TrimSpace(cfg.Addr),
		Username: strings.TrimSpace(cfg.Username),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}

func NewPrincipalCache(client *redis.Client, cfg config.RedisConfig) *PrincipalCache {
	if client == nil {
		return nil
	}

	ttl := cfg.PrincipalTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	return &PrincipalCache{
		client: client,
		prefix: normalizePrefix(cfg.KeyPrefix),
		ttl:    ttl,
	}
}

func (c *PrincipalCache) Get(ctx context.Context, tenantID, userID string, version int) (domainauth.Principal, bool, error) {
	if c == nil || c.client == nil {
		return domainauth.Principal{}, false, nil
	}

	payload, err := c.client.Get(ctx, c.userVersionKey(tenantID, userID, version)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return domainauth.Principal{}, false, nil
		}
		return domainauth.Principal{}, false, fmt.Errorf("get principal cache: %w", err)
	}

	var principal domainauth.Principal
	if err := json.Unmarshal(payload, &principal); err != nil {
		return domainauth.Principal{}, false, fmt.Errorf("decode principal cache: %w", err)
	}
	return principal, true, nil
}

func (c *PrincipalCache) Set(ctx context.Context, principal domainauth.Principal) error {
	if c == nil || c.client == nil {
		return nil
	}

	payload, err := json.Marshal(principal)
	if err != nil {
		return fmt.Errorf("encode principal cache: %w", err)
	}

	if err := c.client.Set(ctx, c.userVersionKey(principal.TenantID, principal.UserID, principal.Version), payload, c.ttl).Err(); err != nil {
		return fmt.Errorf("set principal cache: %w", err)
	}
	return nil
}

func (c *PrincipalCache) InvalidateUser(ctx context.Context, tenantID, userID string) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.deleteByPattern(ctx, c.userPattern(tenantID, userID))
}

func (c *PrincipalCache) InvalidateTenant(ctx context.Context, tenantID string) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.deleteByPattern(ctx, c.tenantPattern(tenantID))
}

func (c *PrincipalCache) deleteByPattern(ctx context.Context, pattern string) error {
	var cursor uint64

	for {
		keys, next, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("scan principal cache keys: %w", err)
		}

		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("delete principal cache keys: %w", err)
			}
		}

		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func (c *PrincipalCache) userVersionKey(tenantID, userID string, version int) string {
	return fmt.Sprintf("%s:auth:principal:%s:%s:v%d", c.prefix, normalizeSegment(tenantID), normalizeSegment(userID), version)
}

func (c *PrincipalCache) userPattern(tenantID, userID string) string {
	return fmt.Sprintf("%s:auth:principal:%s:%s:*", c.prefix, normalizeSegment(tenantID), normalizeSegment(userID))
}

func (c *PrincipalCache) tenantPattern(tenantID string) string {
	return fmt.Sprintf("%s:auth:principal:%s:*", c.prefix, normalizeSegment(tenantID))
}

func normalizePrefix(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, ":")
	if trimmed == "" {
		return "playground"
	}
	return trimmed
}

func normalizeSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "default"
	}
	return trimmed
}

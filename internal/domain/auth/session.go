package auth

import (
	"context"
	"time"
)

type RefreshSession struct {
	UserID    string    `json:"userId"`
	TenantID  string    `json:"tenantId"`
	Username  string    `json:"username"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
}

type RefreshSessionStore interface {
	Create(ctx context.Context, session RefreshSession, ttl time.Duration) (string, time.Time, error)
	Get(ctx context.Context, token string) (RefreshSession, bool, error)
	Delete(ctx context.Context, token string) error
}

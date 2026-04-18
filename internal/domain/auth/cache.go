package auth

import "context"

type PrincipalCache interface {
	Get(ctx context.Context, tenantID, userID string, version int) (Principal, bool, error)
	Set(ctx context.Context, principal Principal) error
}

type PrincipalInvalidator interface {
	InvalidateUser(ctx context.Context, tenantID, userID string) error
	InvalidateTenant(ctx context.Context, tenantID string) error
}

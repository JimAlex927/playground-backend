package auth

import (
	"context"

	domainaccount "playground/internal/domain/account"
	domainauth "playground/internal/domain/auth"
	domainevents "playground/internal/domain/events"
	domainrole "playground/internal/domain/role"
)

type PrincipalCacheInvalidationHandler struct {
	invalidator domainauth.PrincipalInvalidator
}

func NewPrincipalCacheInvalidationHandler(invalidator domainauth.PrincipalInvalidator) *PrincipalCacheInvalidationHandler {
	if invalidator == nil {
		return nil
	}
	return &PrincipalCacheInvalidationHandler{invalidator: invalidator}
}

func (h *PrincipalCacheInvalidationHandler) Handle(ctx context.Context, event domainevents.Event) error {
	if h == nil || h.invalidator == nil || event == nil {
		return nil
	}

	switch item := event.(type) {
	case domainaccount.ProfileUpdated:
		return h.invalidator.InvalidateUser(ctx, item.TenantID, item.AccountID)
	case domainaccount.PasswordChanged:
		return h.invalidator.InvalidateUser(ctx, item.TenantID, item.AccountID)
	case domainaccount.Deleted:
		return h.invalidator.InvalidateUser(ctx, item.TenantID, item.AccountID)
	case domainrole.Updated:
		return h.invalidator.InvalidateTenant(ctx, item.TenantID)
	default:
		return nil
	}
}

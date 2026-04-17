package role

import (
	"errors"
	"slices"
	"strings"
)

const (
	PermissionAccountsRead      = "accounts:read"
	PermissionAccountsWrite     = "accounts:write"
	PermissionCredentialsRead   = "credentials:read"
	PermissionCredentialsWrite  = "credentials:write"
	PermissionCredentialsReveal = "credentials:reveal"
)

func DefaultPermissionCodes() []string {
	return []string{
		PermissionAccountsRead,
		PermissionAccountsWrite,
		PermissionCredentialsRead,
		PermissionCredentialsWrite,
		PermissionCredentialsReveal,
	}
}

func NormalizeAssignedPermissions(values []string) ([]string, error) {
	items := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}

	slices.Sort(items)
	items = slices.Compact(items)
	if len(items) == 0 {
		return nil, errors.New("at least one permission is required")
	}
	return items, nil
}

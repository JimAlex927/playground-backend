package role

import (
	"errors"
	"strings"
	"time"
)

type Role struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenantId"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Permissions []string  `json:"permissions,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateParams struct {
	ID          string
	TenantID    string
	Name        string
	Description string
	Permissions []string
	Now         time.Time
}

type UpdateParams struct {
	Name        string
	Description string
	Permissions []string
	Now         time.Time
}

func New(params CreateParams) (Role, error) {
	if strings.TrimSpace(params.ID) == "" {
		return Role{}, errors.New("role id is required")
	}

	name, err := normalizeName(params.Name)
	if err != nil {
		return Role{}, err
	}

	permissions, err := NormalizeAssignedPermissions(params.Permissions)
	if err != nil {
		return Role{}, err
	}

	now := params.Now.UTC()
	return Role{
		ID:          strings.TrimSpace(params.ID),
		TenantID:    normalizeTenantID(params.TenantID),
		Name:        name,
		Description: normalizeDescription(params.Description),
		Permissions: permissions,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (r *Role) Update(params UpdateParams) error {
	name, err := normalizeName(params.Name)
	if err != nil {
		return err
	}

	permissions, err := NormalizeAssignedPermissions(params.Permissions)
	if err != nil {
		return err
	}

	r.Name = name
	r.Description = normalizeDescription(params.Description)
	r.Permissions = permissions
	r.UpdatedAt = params.Now.UTC()
	return nil
}

func normalizeTenantID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "default"
	}
	return strings.ToLower(trimmed)
}

func normalizeName(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	switch {
	case trimmed == "":
		return "", errors.New("role name is required")
	case len(trimmed) > 64:
		return "", errors.New("role name must be 64 characters or fewer")
	default:
		return strings.ToLower(trimmed), nil
	}
}

func normalizeDescription(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > 255 {
		return trimmed[:255]
	}
	return trimmed
}

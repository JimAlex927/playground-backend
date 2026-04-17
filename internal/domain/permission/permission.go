package permission

import (
	"errors"
	"strings"
	"time"
)

type Permission struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenantId"`
	Code        string    `json:"code"`
	DisplayName string    `json:"displayName"`
	Description string    `json:"description,omitempty"`
	System      bool      `json:"system"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateParams struct {
	ID          string
	TenantID    string
	Code        string
	DisplayName string
	Description string
	System      bool
	Now         time.Time
}

type UpdateParams struct {
	DisplayName string
	Description string
	Now         time.Time
}

func New(params CreateParams) (Permission, error) {
	if strings.TrimSpace(params.ID) == "" {
		return Permission{}, errors.New("permission id is required")
	}

	code, err := normalizeCode(params.Code)
	if err != nil {
		return Permission{}, err
	}
	displayName, err := normalizeDisplayName(params.DisplayName)
	if err != nil {
		return Permission{}, err
	}

	now := params.Now.UTC()
	return Permission{
		ID:          strings.TrimSpace(params.ID),
		TenantID:    normalizeTenantID(params.TenantID),
		Code:        code,
		DisplayName: displayName,
		Description: normalizeDescription(params.Description),
		System:      params.System,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (p *Permission) Update(params UpdateParams) error {
	displayName, err := normalizeDisplayName(params.DisplayName)
	if err != nil {
		return err
	}

	p.DisplayName = displayName
	p.Description = normalizeDescription(params.Description)
	p.UpdatedAt = params.Now.UTC()
	return nil
}

func normalizeCode(value string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	switch {
	case trimmed == "":
		return "", errors.New("permission code is required")
	case len(trimmed) > 64:
		return "", errors.New("permission code must be 64 characters or fewer")
	case !strings.Contains(trimmed, ":") && !strings.Contains(trimmed, "."):
		return "", errors.New("permission code should include a namespace like accounts:read")
	default:
		return trimmed, nil
	}
}

func normalizeDisplayName(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	switch {
	case trimmed == "":
		return "", errors.New("permission display name is required")
	case len(trimmed) > 64:
		return "", errors.New("permission display name must be 64 characters or fewer")
	default:
		return trimmed, nil
	}
}

func normalizeDescription(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > 255 {
		return trimmed[:255]
	}
	return trimmed
}

func normalizeTenantID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "default"
	}
	return strings.ToLower(trimmed)
}

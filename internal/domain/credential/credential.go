package credential

import (
	"errors"
	"strings"
	"time"
)

type Credential struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenantId"`
	OwnerAccountID   string    `json:"ownerAccountId"`
	Title            string    `json:"title"`
	Username         string    `json:"username"`
	Website          string    `json:"website,omitempty"`
	Category         string    `json:"category,omitempty"`
	Notes            string    `json:"notes,omitempty"`
	PasswordEnvelope string    `json:"-"`
	CreatedBy        string    `json:"createdBy,omitempty"`
	UpdatedBy        string    `json:"updatedBy,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type CreateParams struct {
	ID               string
	TenantID         string
	OwnerAccountID   string
	Title            string
	Username         string
	Website          string
	Category         string
	Notes            string
	PasswordEnvelope string
	Actor            string
	Now              time.Time
}

type UpdateParams struct {
	Title            string
	Username         string
	Website          string
	Category         string
	Notes            string
	PasswordEnvelope *string
	Actor            string
	Now              time.Time
}

func New(params CreateParams) (Credential, error) {
	now := params.Now.UTC()
	if strings.TrimSpace(params.ID) == "" {
		return Credential{}, errors.New("credential id is required")
	}

	title, err := normalizeTitle(params.Title)
	if err != nil {
		return Credential{}, err
	}

	username, err := normalizeUsername(params.Username)
	if err != nil {
		return Credential{}, err
	}

	if strings.TrimSpace(params.PasswordEnvelope) == "" {
		return Credential{}, errors.New("password is required")
	}

	if normalizeOwnerAccountID(params.OwnerAccountID) == "" {
		return Credential{}, errors.New("owner account is required")
	}

	return Credential{
		ID:               strings.TrimSpace(params.ID),
		TenantID:         normalizeTenantID(params.TenantID),
		OwnerAccountID:   normalizeOwnerAccountID(params.OwnerAccountID),
		Title:            title,
		Username:         username,
		Website:          normalizeOptional(params.Website, 255),
		Category:         normalizeOptional(params.Category, 64),
		Notes:            normalizeOptional(params.Notes, 4000),
		PasswordEnvelope: strings.TrimSpace(params.PasswordEnvelope),
		CreatedBy:        strings.TrimSpace(params.Actor),
		UpdatedBy:        strings.TrimSpace(params.Actor),
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (c *Credential) Update(params UpdateParams) error {
	title, err := normalizeTitle(params.Title)
	if err != nil {
		return err
	}

	username, err := normalizeUsername(params.Username)
	if err != nil {
		return err
	}

	c.Title = title
	c.Username = username
	c.Website = normalizeOptional(params.Website, 255)
	c.Category = normalizeOptional(params.Category, 64)
	c.Notes = normalizeOptional(params.Notes, 4000)
	c.UpdatedBy = strings.TrimSpace(params.Actor)
	c.UpdatedAt = params.Now.UTC()

	if params.PasswordEnvelope != nil {
		if strings.TrimSpace(*params.PasswordEnvelope) == "" {
			return errors.New("password is required")
		}
		c.PasswordEnvelope = strings.TrimSpace(*params.PasswordEnvelope)
	}

	return nil
}

func normalizeTitle(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	switch {
	case trimmed == "":
		return "", errors.New("title is required")
	case len(trimmed) > 120:
		return "", errors.New("title must be 120 characters or fewer")
	default:
		return trimmed, nil
	}
}

func normalizeUsername(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	switch {
	case trimmed == "":
		return "", errors.New("username is required")
	case len(trimmed) > 120:
		return "", errors.New("username must be 120 characters or fewer")
	default:
		return trimmed, nil
	}
}

func normalizeOptional(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > limit {
		return trimmed[:limit]
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

func normalizeOwnerAccountID(value string) string {
	return strings.TrimSpace(value)
}

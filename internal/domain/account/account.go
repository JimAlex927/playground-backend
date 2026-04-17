package account

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,32}$`)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

type Account struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenantId"`
	Username     string    `json:"username"`
	Email        string    `json:"email,omitempty"`
	DisplayName  string    `json:"displayName,omitempty"`
	PasswordHash string    `json:"passwordHash"`
	RoleID       string    `json:"roleId"`
	RoleName     string    `json:"roleName"`
	Roles        []string  `json:"roles,omitempty"`
	Permissions  []string  `json:"permissions,omitempty"`
	Status       Status    `json:"status"`
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type CreateParams struct {
	ID           string
	TenantID     string
	Username     string
	Email        string
	DisplayName  string
	PasswordHash string
	RoleID       string
	RoleName     string
	Status       Status
	Permissions  []string
	Now          time.Time
}

type UpdateProfileParams struct {
	Username    string
	Email       string
	DisplayName string
	RoleID      string
	RoleName    string
	Permissions []string
	Status      Status
	Now         time.Time
}

func New(params CreateParams) (Account, error) {
	if strings.TrimSpace(params.ID) == "" {
		return Account{}, errors.New("account id is required")
	}

	now := params.Now.UTC()
	status, err := normalizeStatus(params.Status, StatusActive)
	if err != nil {
		return Account{}, err
	}

	normalizedUsername, err := NormalizeUsername(params.Username)
	if err != nil {
		return Account{}, err
	}

	normalizedEmail, err := NormalizeEmail(params.Email)
	if err != nil {
		return Account{}, err
	}

	if strings.TrimSpace(params.PasswordHash) == "" {
		return Account{}, errors.New("password hash is required")
	}
	if strings.TrimSpace(params.RoleID) == "" {
		return Account{}, errors.New("role id is required")
	}
	if strings.TrimSpace(params.RoleName) == "" {
		return Account{}, errors.New("role name is required")
	}

	return Account{
		ID:           strings.TrimSpace(params.ID),
		TenantID:     NormalizeTenantID(params.TenantID),
		Username:     normalizedUsername,
		Email:        normalizedEmail,
		DisplayName:  strings.TrimSpace(params.DisplayName),
		PasswordHash: params.PasswordHash,
		RoleID:       strings.TrimSpace(params.RoleID),
		RoleName:     strings.TrimSpace(params.RoleName),
		Roles:        roleNames(params.RoleName),
		Permissions:  normalizeList(params.Permissions),
		Status:       status,
		Version:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (a *Account) UpdateProfile(params UpdateProfileParams) error {
	normalizedUsername, err := NormalizeUsername(params.Username)
	if err != nil {
		return err
	}

	normalizedEmail, err := NormalizeEmail(params.Email)
	if err != nil {
		return err
	}

	status, err := normalizeStatus(params.Status, a.Status)
	if err != nil {
		return err
	}
	if strings.TrimSpace(params.RoleID) == "" {
		return errors.New("role id is required")
	}
	if strings.TrimSpace(params.RoleName) == "" {
		return errors.New("role name is required")
	}

	a.Username = normalizedUsername
	a.Email = normalizedEmail
	a.DisplayName = strings.TrimSpace(params.DisplayName)
	a.RoleID = strings.TrimSpace(params.RoleID)
	a.RoleName = strings.TrimSpace(params.RoleName)
	a.Roles = roleNames(params.RoleName)
	a.Permissions = normalizeList(params.Permissions)
	a.Status = status
	/**
	这里是一个坑 如果更新了profile 这个version会+1
	而version+1 会导致token失效 但是profile 不会把密码也修改
	密码是走的password的接口
	也就是点击修改密码 会先触发put 导致token失效 token失效再去修改密码就修改不了了 所以这里不能把version加上去
	*/
	//TODO  修改profile不能修改version
	//a.Version++
	a.UpdatedAt = params.Now.UTC()

	return nil
}

func (a *Account) ChangePassword(passwordHash string, now time.Time) error {
	if strings.TrimSpace(passwordHash) == "" {
		return errors.New("password hash is required")
	}

	a.PasswordHash = passwordHash
	a.Version++
	a.UpdatedAt = now.UTC()
	return nil
}

func (a Account) CanLogin() bool {
	return a.Status == StatusActive
}

func NormalizeUsername(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if !usernamePattern.MatchString(normalized) {
		return "", errors.New("username must be 3-32 chars and contain only letters, numbers, dot, underscore or hyphen")
	}
	return strings.ToLower(normalized), nil
}

func NormalizeEmail(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}

	normalized := strings.ToLower(trimmed)
	if !strings.Contains(normalized, "@") || strings.HasPrefix(normalized, "@") || strings.HasSuffix(normalized, "@") {
		return "", errors.New("email format is invalid")
	}

	return normalized, nil
}

func NormalizeTenantID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "default"
	}
	return strings.ToLower(trimmed)
}

func NormalizeLoginID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeStatus(value Status, fallback Status) (Status, error) {
	status := value
	if status == "" {
		status = fallback
	}

	switch status {
	case StatusActive, StatusDisabled:
		return status, nil
	default:
		return "", errors.New("status must be active or disabled")
	}
}

func normalizeList(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	items := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		items = append(items, trimmed)
	}

	slices.Sort(items)
	return slices.Compact(items)
}

func roleNames(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return []string{trimmed}
}

package account

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	domainaccount "playground/internal/domain/account"
	domainrole "playground/internal/domain/role"
)

type Repository interface {
	List(ctx context.Context) ([]domainaccount.Account, error)
	ListPage(ctx context.Context, query ListQuery) (PageResult, error)
	GetByID(ctx context.Context, id string) (domainaccount.Account, error)
	FindByLoginID(ctx context.Context, loginID string) (domainaccount.Account, error)
	Create(ctx context.Context, item domainaccount.Account) error
	Update(ctx context.Context, item domainaccount.Account) error
	Delete(ctx context.Context, id string) error
	CountByRoleID(ctx context.Context, tenantID, roleID string) (int, error)
}

type RoleReader interface {
	GetByID(ctx context.Context, tenantID, id string) (domainrole.Role, error)
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(encodedHash, password string) error
}

type Service struct {
	repo     Repository
	roles    RoleReader
	hasher   PasswordHasher
	now      func() time.Time
	idSource func() string
}

type CreateAccountInput struct {
	TenantID    string `json:"tenantId"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
	RoleID      string `json:"roleId"`
	Status      string `json:"status"`
}

type UpdateAccountInput struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	RoleID      string `json:"roleId"`
	Status      string `json:"status"`
}

type ChangePasswordInput struct {
	Password string `json:"password"`
}

type ListQuery struct {
	TenantID string
	Keyword  string
	Page     int
	PageSize int
}

type PageResult struct {
	Items      []domainaccount.Account
	Page       int
	PageSize   int
	Total      int
	TotalPages int
}

func NewService(repo Repository, roles RoleReader, hasher PasswordHasher, now func() time.Time) Service {
	return Service{
		repo:     repo,
		roles:    roles,
		hasher:   hasher,
		now:      now,
		idSource: newAccountID,
	}
}

func (s Service) ListAccounts(ctx context.Context) ([]domainaccount.Account, error) {
	return s.repo.List(ctx)
}

func (s Service) ListAccountPage(ctx context.Context, query ListQuery) (PageResult, error) {
	query.TenantID = normalizeTenantID(query.TenantID)
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Page = normalizePage(query.Page)
	query.PageSize = normalizePageSize(query.PageSize)
	return s.repo.ListPage(ctx, query)
}

func (s Service) GetAccount(ctx context.Context, id string) (domainaccount.Account, error) {
	return s.repo.GetByID(ctx, strings.TrimSpace(id))
}

func (s Service) CreateAccount(ctx context.Context, input CreateAccountInput) (domainaccount.Account, error) {
	if len(strings.TrimSpace(input.Password)) < 8 {
		return domainaccount.Account{}, errors.New("password must be at least 8 characters")
	}

	passwordHash, err := s.hasher.Hash(input.Password)
	if err != nil {
		return domainaccount.Account{}, fmt.Errorf("hash password: %w", err)
	}

	role, err := s.roles.GetByID(ctx, normalizeTenantID(input.TenantID), strings.TrimSpace(input.RoleID))
	if err != nil {
		return domainaccount.Account{}, err
	}

	status := domainaccount.Status(strings.TrimSpace(input.Status))
	item, err := domainaccount.New(domainaccount.CreateParams{
		ID:           s.idSource(),
		TenantID:     input.TenantID,
		Username:     input.Username,
		Email:        input.Email,
		DisplayName:  input.DisplayName,
		PasswordHash: passwordHash,
		RoleID:       role.ID,
		RoleName:     role.Name,
		Permissions:  role.Permissions,
		Status:       status,
		Now:          s.now(),
	})
	if err != nil {
		return domainaccount.Account{}, err
	}

	if err := s.repo.Create(ctx, item); err != nil {
		return domainaccount.Account{}, err
	}

	return item, nil
}

func (s Service) UpdateAccount(ctx context.Context, id string, input UpdateAccountInput) (domainaccount.Account, error) {
	item, err := s.repo.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return domainaccount.Account{}, err
	}

	role, err := s.roles.GetByID(ctx, item.TenantID, strings.TrimSpace(input.RoleID))
	if err != nil {
		return domainaccount.Account{}, err
	}

	if err := item.UpdateProfile(domainaccount.UpdateProfileParams{
		Username:    input.Username,
		Email:       input.Email,
		DisplayName: input.DisplayName,
		RoleID:      role.ID,
		RoleName:    role.Name,
		Permissions: role.Permissions,
		Status:      domainaccount.Status(strings.TrimSpace(input.Status)),
		Now:         s.now(),
	}); err != nil {
		return domainaccount.Account{}, err
	}

	if err := s.repo.Update(ctx, item); err != nil {
		return domainaccount.Account{}, err
	}

	return item, nil
}

func (s Service) ChangePassword(ctx context.Context, id string, input ChangePasswordInput) (domainaccount.Account, error) {
	if len(strings.TrimSpace(input.Password)) < 8 {
		return domainaccount.Account{}, errors.New("password must be at least 8 characters")
	}

	item, err := s.repo.GetByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return domainaccount.Account{}, err
	}

	passwordHash, err := s.hasher.Hash(input.Password)
	if err != nil {
		return domainaccount.Account{}, fmt.Errorf("hash password: %w", err)
	}

	if err := item.ChangePassword(passwordHash, s.now()); err != nil {
		return domainaccount.Account{}, err
	}

	if err := s.repo.Update(ctx, item); err != nil {
		return domainaccount.Account{}, err
	}

	return item, nil
}

func (s Service) DeleteAccount(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, strings.TrimSpace(id))
}

func (s Service) EnsureBootstrapAdmin(ctx context.Context, input CreateAccountInput) (domainaccount.Account, bool, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return domainaccount.Account{}, false, err
	}

	if len(items) > 0 {
		return domainaccount.Account{}, false, nil
	}

	created, err := s.CreateAccount(ctx, input)
	if err != nil {
		return domainaccount.Account{}, false, err
	}

	return created, true, nil
}

func newAccountID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("acct_%d", time.Now().UnixNano())
	}
	return "acct_" + hex.EncodeToString(buf)
}

func normalizeTenantID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "default"
	}
	return strings.ToLower(trimmed)
}

func normalizePage(value int) int {
	if value < 1 {
		return 1
	}
	return value
}

func normalizePageSize(value int) int {
	switch {
	case value <= 0:
		return 10
	case value > 50:
		return 50
	default:
		return value
	}
}

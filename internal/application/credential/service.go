package credential

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"

	domaincredential "playground/internal/domain/credential"
)

type Repository interface {
	List(ctx context.Context, query ListQuery) (PageResult, error)
	GetByID(ctx context.Context, tenantID, ownerAccountID, id string) (domaincredential.Credential, error)
	Create(ctx context.Context, item domaincredential.Credential) error
	Update(ctx context.Context, item domaincredential.Credential) error
	Delete(ctx context.Context, tenantID, ownerAccountID, id string) error
}

type Cipher interface {
	Encrypt(value string) (string, error)
	Decrypt(value string) (string, error)
}

type Service struct {
	repo     Repository
	cipher   Cipher
	now      func() time.Time
	idSource func() string
}

type ListQuery struct {
	TenantID       string
	OwnerAccountID string
	Keyword        string
	Page           int
	PageSize       int
}

type PageResult struct {
	Items      []domaincredential.Credential
	Page       int
	PageSize   int
	Total      int
	TotalPages int
}

type CreateInput struct {
	TenantID       string `json:"tenantId"`
	OwnerAccountID string `json:"ownerAccountId"`
	Title          string `json:"title"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	Website        string `json:"website"`
	Category       string `json:"category"`
	Notes          string `json:"notes"`
	Actor          string `json:"actor"`
}

type UpdateInput struct {
	TenantID string `json:"tenantId"`
	Title    string `json:"title"`
	Username string `json:"username"`
	Password string `json:"password"`
	Website  string `json:"website"`
	Category string `json:"category"`
	Notes    string `json:"notes"`
	Actor    string `json:"actor"`
}

func NewService(repo Repository, cipher Cipher, now func() time.Time) Service {
	return Service{
		repo:     repo,
		cipher:   cipher,
		now:      now,
		idSource: newCredentialID,
	}
}

func (s Service) ListCredentials(ctx context.Context, query ListQuery) (PageResult, error) {
	query.TenantID = normalizeTenantID(query.TenantID)
	query.OwnerAccountID = normalizeOwnerAccountID(query.OwnerAccountID)
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Page = normalizePage(query.Page)
	query.PageSize = normalizePageSize(query.PageSize)
	return s.repo.List(ctx, query)
}

func (s Service) GetCredential(ctx context.Context, tenantID, ownerAccountID, id string) (domaincredential.Credential, error) {
	return s.repo.GetByID(ctx, normalizeTenantID(tenantID), normalizeOwnerAccountID(ownerAccountID), strings.TrimSpace(id))
}

func (s Service) CreateCredential(ctx context.Context, input CreateInput) (domaincredential.Credential, error) {
	envelope, err := s.cipher.Encrypt(strings.TrimSpace(input.Password))
	if err != nil {
		return domaincredential.Credential{}, fmt.Errorf("encrypt credential password: %w", err)
	}

	item, err := domaincredential.New(domaincredential.CreateParams{
		ID:               s.idSource(),
		TenantID:         input.TenantID,
		OwnerAccountID:   input.OwnerAccountID,
		Title:            input.Title,
		Username:         input.Username,
		Website:          input.Website,
		Category:         input.Category,
		Notes:            input.Notes,
		PasswordEnvelope: envelope,
		Actor:            input.Actor,
		Now:              s.now(),
	})
	if err != nil {
		return domaincredential.Credential{}, err
	}

	if err := s.repo.Create(ctx, item); err != nil {
		return domaincredential.Credential{}, err
	}

	return item, nil
}

func (s Service) UpdateCredential(ctx context.Context, tenantID, ownerAccountID, id string, input UpdateInput) (domaincredential.Credential, error) {
	item, err := s.repo.GetByID(ctx, normalizeTenantID(tenantID), normalizeOwnerAccountID(ownerAccountID), strings.TrimSpace(id))
	if err != nil {
		return domaincredential.Credential{}, err
	}

	var passwordEnvelope *string
	if strings.TrimSpace(input.Password) != "" {
		encrypted, err := s.cipher.Encrypt(strings.TrimSpace(input.Password))
		if err != nil {
			return domaincredential.Credential{}, fmt.Errorf("encrypt credential password: %w", err)
		}
		passwordEnvelope = &encrypted
	}

	if err := item.Update(domaincredential.UpdateParams{
		Title:            input.Title,
		Username:         input.Username,
		Website:          input.Website,
		Category:         input.Category,
		Notes:            input.Notes,
		PasswordEnvelope: passwordEnvelope,
		Actor:            input.Actor,
		Now:              s.now(),
	}); err != nil {
		return domaincredential.Credential{}, err
	}

	if err := s.repo.Update(ctx, item); err != nil {
		return domaincredential.Credential{}, err
	}

	return item, nil
}

func (s Service) DeleteCredential(ctx context.Context, tenantID, ownerAccountID, id string) error {
	return s.repo.Delete(ctx, normalizeTenantID(tenantID), normalizeOwnerAccountID(ownerAccountID), strings.TrimSpace(id))
}

func (s Service) RevealPassword(ctx context.Context, tenantID, ownerAccountID, id string) (string, error) {
	item, err := s.repo.GetByID(ctx, normalizeTenantID(tenantID), normalizeOwnerAccountID(ownerAccountID), strings.TrimSpace(id))
	if err != nil {
		return "", err
	}

	secret, err := s.cipher.Decrypt(item.PasswordEnvelope)
	if err != nil {
		return "", fmt.Errorf("decrypt credential password: %w", err)
	}

	return secret, nil
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

func TotalPages(total, pageSize int) int {
	if total == 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}

func newCredentialID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("cred_%d", time.Now().UnixNano())
	}
	return "cred_" + hex.EncodeToString(buf)
}

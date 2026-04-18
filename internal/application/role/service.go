package role

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	domainevents "playground/internal/domain/events"
	domainrole "playground/internal/domain/role"
)

type Repository interface {
	List(ctx context.Context, tenantID string) ([]domainrole.Role, error)
	GetByID(ctx context.Context, tenantID, id string) (domainrole.Role, error)
	FindByName(ctx context.Context, tenantID, name string) (domainrole.Role, error)
	Create(ctx context.Context, item domainrole.Role) error
	Update(ctx context.Context, item domainrole.Role) error
	Delete(ctx context.Context, tenantID, id string) error
}

type AccountUsageReader interface {
	CountByRoleID(ctx context.Context, tenantID, roleID string) (int, error)
}

type PermissionResolver interface {
	ResolveCodes(ctx context.Context, tenantID string, codes []string) ([]string, error)
}

type Service struct {
	repo        Repository
	accountUses AccountUsageReader
	permissions PermissionResolver
	publisher   domainevents.Publisher
	now         func() time.Time
	idSource    func() string
}

type CreateInput struct {
	TenantID    string   `json:"tenantId"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

type UpdateInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func NewService(repo Repository, accountUses AccountUsageReader, permissions PermissionResolver, publisher domainevents.Publisher, now func() time.Time) Service {
	return Service{
		repo:        repo,
		accountUses: accountUses,
		permissions: permissions,
		publisher:   publisher,
		now:         now,
		idSource:    newRoleID,
	}
}

func (s Service) ListRoles(ctx context.Context, tenantID string) ([]domainrole.Role, error) {
	return s.repo.List(ctx, normalizeTenantID(tenantID))
}

func (s Service) GetRole(ctx context.Context, tenantID, id string) (domainrole.Role, error) {
	return s.repo.GetByID(ctx, normalizeTenantID(tenantID), strings.TrimSpace(id))
}

func (s Service) FindRoleByName(ctx context.Context, tenantID, name string) (domainrole.Role, error) {
	return s.repo.FindByName(ctx, normalizeTenantID(tenantID), strings.TrimSpace(name))
}

func (s Service) CreateRole(ctx context.Context, input CreateInput) (domainrole.Role, error) {
	permissionCodes, err := s.permissions.ResolveCodes(ctx, normalizeTenantID(input.TenantID), input.Permissions)
	if err != nil {
		return domainrole.Role{}, err
	}

	item, err := domainrole.New(domainrole.CreateParams{
		ID:          s.idSource(),
		TenantID:    input.TenantID,
		Name:        input.Name,
		Description: input.Description,
		Permissions: permissionCodes,
		Now:         s.now(),
	})
	if err != nil {
		return domainrole.Role{}, err
	}

	if err := s.repo.Create(ctx, item); err != nil {
		return domainrole.Role{}, err
	}
	return item, nil
}

func (s Service) UpdateRole(ctx context.Context, tenantID, id string, input UpdateInput) (domainrole.Role, error) {
	item, err := s.repo.GetByID(ctx, normalizeTenantID(tenantID), strings.TrimSpace(id))
	if err != nil {
		return domainrole.Role{}, err
	}

	permissionCodes, err := s.permissions.ResolveCodes(ctx, item.TenantID, input.Permissions)
	if err != nil {
		return domainrole.Role{}, err
	}

	if err := item.Update(domainrole.UpdateParams{
		Name:        input.Name,
		Description: input.Description,
		Permissions: permissionCodes,
		Now:         s.now(),
	}); err != nil {
		return domainrole.Role{}, err
	}

	if err := s.repo.Update(ctx, item); err != nil {
		return domainrole.Role{}, err
	}
	if s.publisher != nil {
		if err := s.publisher.Publish(ctx, item.PullEvents()...); err != nil {
			return domainrole.Role{}, err
		}
	}
	return item, nil
}

func (s Service) DeleteRole(ctx context.Context, tenantID, id string) error {
	normalizedTenant := normalizeTenantID(tenantID)
	normalizedID := strings.TrimSpace(id)
	if s.accountUses != nil {
		total, err := s.accountUses.CountByRoleID(ctx, normalizedTenant, normalizedID)
		if err != nil {
			return err
		}
		if total > 0 {
			return fmt.Errorf("role is assigned to %d users", total)
		}
	}
	if err := s.repo.Delete(ctx, normalizedTenant, normalizedID); err != nil {
		return err
	}
	return nil
}

func (s Service) EnsureDefaultRoles(ctx context.Context, tenantID string) (map[string]domainrole.Role, error) {
	type preset struct {
		name        string
		description string
		permissions []string
	}

	presets := []preset{
		{
			name:        "master",
			description: "系统超级管理员，拥有后台与密码库完整能力。",
			permissions: domainrole.DefaultPermissionCodes(),
		},
		{
			name:        "member",
			description: "密码库成员，可管理自己的密码条目。",
			permissions: []string{domainrole.PermissionCredentialsRead, domainrole.PermissionCredentialsWrite, domainrole.PermissionCredentialsReveal},
		},
		{
			name:        "viewer",
			description: "只读成员，仅可查看自己的密码条目。",
			permissions: []string{domainrole.PermissionCredentialsRead},
		},
	}

	result := make(map[string]domainrole.Role, len(presets))
	for _, preset := range presets {
		item, err := s.repo.FindByName(ctx, normalizeTenantID(tenantID), preset.name)
		if err == nil {
			result[preset.name] = item
			continue
		}
		if err != nil && err != domainrole.ErrNotFound {
			return nil, err
		}

		created, err := s.CreateRole(ctx, CreateInput{
			TenantID:    tenantID,
			Name:        preset.name,
			Description: preset.description,
			Permissions: preset.permissions,
		})
		if err != nil {
			return nil, err
		}
		result[preset.name] = created
	}

	return result, nil
}

func normalizeTenantID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "default"
	}
	return strings.ToLower(trimmed)
}

func newRoleID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("role_%d", time.Now().UnixNano())
	}
	return "role_" + hex.EncodeToString(buf)
}

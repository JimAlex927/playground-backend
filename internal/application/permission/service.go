package permission

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	domainpermission "playground/internal/domain/permission"
	domainrole "playground/internal/domain/role"
)

type Repository interface {
	List(ctx context.Context, tenantID string) ([]domainpermission.Permission, error)
	GetByID(ctx context.Context, tenantID, id string) (domainpermission.Permission, error)
	GetByCode(ctx context.Context, tenantID, code string) (domainpermission.Permission, error)
	Create(ctx context.Context, item domainpermission.Permission) error
	Update(ctx context.Context, item domainpermission.Permission) error
	Delete(ctx context.Context, tenantID, id string) error
	CountRolesByCode(ctx context.Context, tenantID, code string) (int, error)
}

type Service struct {
	repo     Repository
	now      func() time.Time
	idSource func() string
}

type CreateInput struct {
	TenantID    string `json:"tenantId"`
	Code        string `json:"code"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

type UpdateInput struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

func NewService(repo Repository, now func() time.Time) Service {
	return Service{
		repo:     repo,
		now:      now,
		idSource: newPermissionID,
	}
}

func (s Service) ListPermissions(ctx context.Context, tenantID string) ([]domainpermission.Permission, error) {
	return s.repo.List(ctx, normalizeTenantID(tenantID))
}

func (s Service) GetPermission(ctx context.Context, tenantID, id string) (domainpermission.Permission, error) {
	return s.repo.GetByID(ctx, normalizeTenantID(tenantID), strings.TrimSpace(id))
}

func (s Service) CreatePermission(ctx context.Context, input CreateInput) (domainpermission.Permission, error) {
	item, err := domainpermission.New(domainpermission.CreateParams{
		ID:          s.idSource(),
		TenantID:    input.TenantID,
		Code:        input.Code,
		DisplayName: input.DisplayName,
		Description: input.Description,
		Now:         s.now(),
	})
	if err != nil {
		return domainpermission.Permission{}, err
	}

	if err := s.repo.Create(ctx, item); err != nil {
		return domainpermission.Permission{}, err
	}
	return item, nil
}

func (s Service) UpdatePermission(ctx context.Context, tenantID, id string, input UpdateInput) (domainpermission.Permission, error) {
	item, err := s.repo.GetByID(ctx, normalizeTenantID(tenantID), strings.TrimSpace(id))
	if err != nil {
		return domainpermission.Permission{}, err
	}

	if err := item.Update(domainpermission.UpdateParams{
		DisplayName: input.DisplayName,
		Description: input.Description,
		Now:         s.now(),
	}); err != nil {
		return domainpermission.Permission{}, err
	}

	if err := s.repo.Update(ctx, item); err != nil {
		return domainpermission.Permission{}, err
	}
	return item, nil
}

func (s Service) DeletePermission(ctx context.Context, tenantID, id string) error {
	item, err := s.repo.GetByID(ctx, normalizeTenantID(tenantID), strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if item.System {
		return domainpermission.ErrSystemLocked
	}

	total, err := s.repo.CountRolesByCode(ctx, item.TenantID, item.Code)
	if err != nil {
		return err
	}
	if total > 0 {
		return fmt.Errorf("permission is assigned to %d roles", total)
	}

	if err := s.repo.Delete(ctx, item.TenantID, item.ID); err != nil {
		return err
	}
	return nil
}

func (s Service) EnsureDefaultPermissions(ctx context.Context, tenantID string) ([]domainpermission.Permission, error) {
	presets := []struct {
		code        string
		displayName string
		description string
	}{
		{domainrole.PermissionAccountsRead, "查看用户", "允许读取后台用户与角色。"},
		{domainrole.PermissionAccountsWrite, "管理用户", "允许创建、编辑、删除用户和角色。"},
		{domainrole.PermissionCredentialsRead, "查看密码库", "允许读取自己的密码条目。"},
		{domainrole.PermissionCredentialsWrite, "编辑密码库", "允许新增、更新、删除自己的密码条目。"},
		{domainrole.PermissionCredentialsReveal, "查看密码明文", "允许解密并复制密码字段。"},
	}

	items := make([]domainpermission.Permission, 0, len(presets))
	for _, preset := range presets {
		existing, err := s.repo.GetByCode(ctx, normalizeTenantID(tenantID), preset.code)
		if err == nil {
			items = append(items, existing)
			continue
		}
		if err != nil && err != domainpermission.ErrNotFound {
			return nil, err
		}

		created, err := domainpermission.New(domainpermission.CreateParams{
			ID:          s.idSource(),
			TenantID:    tenantID,
			Code:        preset.code,
			DisplayName: preset.displayName,
			Description: preset.description,
			System:      true,
			Now:         s.now(),
		})
		if err != nil {
			return nil, err
		}
		if err := s.repo.Create(ctx, created); err != nil {
			return nil, err
		}
		items = append(items, created)
	}

	return items, nil
}

func (s Service) ResolveCodes(ctx context.Context, tenantID string, codes []string) ([]string, error) {
	normalized, err := domainrole.NormalizeAssignedPermissions(codes)
	if err != nil {
		return nil, err
	}

	items, err := s.repo.List(ctx, normalizeTenantID(tenantID))
	if err != nil {
		return nil, err
	}

	allowed := make(map[string]struct{}, len(items))
	for _, item := range items {
		allowed[item.Code] = struct{}{}
	}

	for _, code := range normalized {
		if _, ok := allowed[code]; !ok {
			return nil, fmt.Errorf("permission %q not found", code)
		}
	}
	return normalized, nil
}

func normalizeTenantID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "default"
	}
	return strings.ToLower(trimmed)
}

func newPermissionID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("perm_%d", time.Now().UnixNano())
	}
	return "perm_" + hex.EncodeToString(buf)
}

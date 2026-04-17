package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"

	domainpermission "playground/internal/domain/permission"
)

type PermissionRepository struct {
	db *sql.DB
}

func NewPermissionRepository(db *sql.DB) *PermissionRepository {
	return &PermissionRepository{db: db}
}

func (r *PermissionRepository) List(ctx context.Context, tenantID string) ([]domainpermission.Permission, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, code, display_name, description, is_system, created_at, updated_at
		FROM permissions
		WHERE tenant_id = ?
		ORDER BY is_system DESC, display_name ASC, code ASC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()

	items := make([]domainpermission.Permission, 0)
	for rows.Next() {
		item, err := scanPermission(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate permissions: %w", err)
	}
	return items, nil
}

func (r *PermissionRepository) GetByID(ctx context.Context, tenantID, id string) (domainpermission.Permission, error) {
	return r.getOne(ctx, `
		SELECT id, tenant_id, code, display_name, description, is_system, created_at, updated_at
		FROM permissions
		WHERE tenant_id = ? AND id = ?
	`, tenantID, strings.TrimSpace(id))
}

func (r *PermissionRepository) GetByCode(ctx context.Context, tenantID, code string) (domainpermission.Permission, error) {
	return r.getOne(ctx, `
		SELECT id, tenant_id, code, display_name, description, is_system, created_at, updated_at
		FROM permissions
		WHERE tenant_id = ? AND code = ?
		LIMIT 1
	`, tenantID, strings.ToLower(strings.TrimSpace(code)))
}

func (r *PermissionRepository) Create(ctx context.Context, item domainpermission.Permission) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO permissions (id, tenant_id, code, display_name, description, is_system, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		item.ID,
		item.TenantID,
		item.Code,
		item.DisplayName,
		nullString(item.Description),
		item.System,
		item.CreatedAt.UTC(),
		item.UpdatedAt.UTC(),
	)
	if err != nil {
		return mapPermissionError(err)
	}
	return nil
}

func (r *PermissionRepository) Update(ctx context.Context, item domainpermission.Permission) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE permissions
		SET display_name = ?, description = ?, updated_at = ?
		WHERE tenant_id = ? AND id = ?
	`,
		item.DisplayName,
		nullString(item.Description),
		item.UpdatedAt.UTC(),
		item.TenantID,
		item.ID,
	)
	if err != nil {
		return fmt.Errorf("update permission: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated permission rows: %w", err)
	}
	if affected == 0 {
		return domainpermission.ErrNotFound
	}
	return nil
}

func (r *PermissionRepository) Delete(ctx context.Context, tenantID, id string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM permissions
		WHERE tenant_id = ? AND id = ?
	`, tenantID, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete permission: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted permission rows: %w", err)
	}
	if affected == 0 {
		return domainpermission.ErrNotFound
	}
	return nil
}

func (r *PermissionRepository) CountRolesByCode(ctx context.Context, tenantID, code string) (int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM role_permissions bindings
		INNER JOIN roles r ON r.id = bindings.role_id
		WHERE r.tenant_id = ? AND bindings.permission_code = ?
	`, tenantID, strings.ToLower(strings.TrimSpace(code))).Scan(&total); err != nil {
		return 0, fmt.Errorf("count roles by permission code: %w", err)
	}
	return total, nil
}

func (r *PermissionRepository) getOne(ctx context.Context, query string, args ...any) (domainpermission.Permission, error) {
	row := r.db.QueryRowContext(ctx, query, args...)
	item, err := scanPermission(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domainpermission.Permission{}, domainpermission.ErrNotFound
		}
		return domainpermission.Permission{}, err
	}
	return item, nil
}

type permissionScanner interface {
	Scan(dest ...any) error
}

func scanPermission(row permissionScanner) (domainpermission.Permission, error) {
	var (
		item        domainpermission.Permission
		description sql.NullString
		createdAt   sql.NullTime
		updatedAt   sql.NullTime
	)
	if err := row.Scan(
		&item.ID,
		&item.TenantID,
		&item.Code,
		&item.DisplayName,
		&description,
		&item.System,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domainpermission.Permission{}, fmt.Errorf("scan permission: %w", err)
	}

	item.Description = strings.TrimSpace(description.String)
	item.CreatedAt = createdAt.Time.UTC()
	item.UpdatedAt = updatedAt.Time.UTC()
	return item, nil
}

func mapPermissionError(err error) error {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		if strings.Contains(strings.ToLower(mysqlErr.Message), "uq_permissions_tenant_code") {
			return domainpermission.ErrDuplicateCode
		}
	}
	return err
}

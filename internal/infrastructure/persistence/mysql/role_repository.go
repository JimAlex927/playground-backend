package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"

	domainrole "playground/internal/domain/role"
)

type RoleRepository struct {
	db *sql.DB
}

func NewRoleRepository(db *sql.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

const roleSelectColumns = `
	SELECT id, tenant_id, name, description, created_at, updated_at
	FROM roles
`

func (r *RoleRepository) List(ctx context.Context, tenantID string) ([]domainrole.Role, error) {
	rows, err := r.db.QueryContext(ctx, roleSelectColumns+`
		WHERE tenant_id = ?
		ORDER BY name ASC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	items := make([]domainrole.Role, 0)
	for rows.Next() {
		item, err := scanRoleBase(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate roles: %w", err)
	}

	if err := r.hydratePermissions(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *RoleRepository) GetByID(ctx context.Context, tenantID, id string) (domainrole.Role, error) {
	item, err := r.getOne(ctx, roleSelectColumns+`
		WHERE tenant_id = ? AND id = ?
	`, tenantID, strings.TrimSpace(id))
	if err != nil {
		return domainrole.Role{}, err
	}
	if err := r.hydrateRole(ctx, &item); err != nil {
		return domainrole.Role{}, err
	}
	return item, nil
}

func (r *RoleRepository) FindByName(ctx context.Context, tenantID, name string) (domainrole.Role, error) {
	item, err := r.getOne(ctx, roleSelectColumns+`
		WHERE tenant_id = ? AND LOWER(name) = ?
		LIMIT 1
	`, tenantID, strings.ToLower(strings.TrimSpace(name)))
	if err != nil {
		return domainrole.Role{}, err
	}
	if err := r.hydrateRole(ctx, &item); err != nil {
		return domainrole.Role{}, err
	}
	return item, nil
}

func (r *RoleRepository) Create(ctx context.Context, item domainrole.Role) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create role: %w", err)
	}
	defer rollbackQuietly(tx)

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO roles (id, tenant_id, name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		item.ID,
		item.TenantID,
		item.Name,
		nullString(item.Description),
		item.CreatedAt.UTC(),
		item.UpdatedAt.UTC(),
	); err != nil {
		return mapRoleError(err)
	}

	if err := replaceRolePermissionsTx(ctx, tx, item.ID, item.Permissions); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit create role: %w", err)
	}
	return nil
}

func (r *RoleRepository) Update(ctx context.Context, item domainrole.Role) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update role: %w", err)
	}
	defer rollbackQuietly(tx)

	result, err := tx.ExecContext(ctx, `
		UPDATE roles
		SET name = ?, description = ?, updated_at = ?
		WHERE tenant_id = ? AND id = ?
	`,
		item.Name,
		nullString(item.Description),
		item.UpdatedAt.UTC(),
		item.TenantID,
		item.ID,
	)
	if err != nil {
		return mapRoleError(err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated role rows: %w", err)
	}
	if affected == 0 {
		return domainrole.ErrNotFound
	}

	if err := replaceRolePermissionsTx(ctx, tx, item.ID, item.Permissions); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update role: %w", err)
	}
	return nil
}

func (r *RoleRepository) Delete(ctx context.Context, tenantID, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete role: %w", err)
	}
	defer rollbackQuietly(tx)

	if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = ?`, strings.TrimSpace(id)); err != nil {
		return fmt.Errorf("delete role permissions: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		DELETE FROM roles
		WHERE tenant_id = ? AND id = ?
	`, tenantID, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted role rows: %w", err)
	}
	if affected == 0 {
		return domainrole.ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete role: %w", err)
	}
	return nil
}

func (r *RoleRepository) getOne(ctx context.Context, query string, args ...any) (domainrole.Role, error) {
	row := r.db.QueryRowContext(ctx, query, args...)
	item, err := scanRoleBase(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domainrole.Role{}, domainrole.ErrNotFound
		}
		return domainrole.Role{}, err
	}
	return item, nil
}

func (r *RoleRepository) hydratePermissions(ctx context.Context, items []domainrole.Role) error {
	roleIDs := make([]string, 0, len(items))
	for _, item := range items {
		roleIDs = append(roleIDs, item.ID)
	}

	metaByID, err := loadRoleMetaByIDs(ctx, r.db, roleIDs)
	if err != nil {
		return err
	}

	for index := range items {
		if meta, ok := metaByID[items[index].ID]; ok {
			items[index].Permissions = meta.Permissions
		}
	}
	return nil
}

func (r *RoleRepository) hydrateRole(ctx context.Context, item *domainrole.Role) error {
	metaByID, err := loadRoleMetaByIDs(ctx, r.db, []string{item.ID})
	if err != nil {
		return err
	}
	if meta, ok := metaByID[item.ID]; ok {
		item.Permissions = meta.Permissions
	}
	return nil
}

type roleScanner interface {
	Scan(dest ...any) error
}

func scanRoleBase(row roleScanner) (domainrole.Role, error) {
	var (
		item        domainrole.Role
		description sql.NullString
		createdAt   sql.NullTime
		updatedAt   sql.NullTime
	)

	if err := row.Scan(
		&item.ID,
		&item.TenantID,
		&item.Name,
		&description,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domainrole.Role{}, fmt.Errorf("scan role: %w", err)
	}

	item.Description = strings.TrimSpace(description.String)
	item.CreatedAt = createdAt.Time.UTC()
	item.UpdatedAt = updatedAt.Time.UTC()
	return item, nil
}

func mapRoleError(err error) error {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		if strings.Contains(strings.ToLower(mysqlErr.Message), "uq_roles_tenant_name") {
			return domainrole.ErrDuplicateName
		}
	}
	return err
}

func rollbackQuietly(tx *sql.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"

	appaccount "playground/internal/application/account"
	"playground/internal/domain/account"
)

type AccountRepository struct {
	db *sql.DB
}

func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

const accountSelectColumns = `
	SELECT
		a.id,
		a.tenant_id,
		a.username,
		a.email,
		a.display_name,
		a.password_hash,
		a.role_id,
		r.name,
		a.status,
		a.version,
		a.created_at,
		a.updated_at
	FROM accounts a
	LEFT JOIN roles r
		ON r.id = a.role_id
		AND r.tenant_id = a.tenant_id
`

func (r *AccountRepository) List(ctx context.Context) ([]account.Account, error) {
	rows, err := r.db.QueryContext(ctx, accountSelectColumns+`
		ORDER BY a.username ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	items := make([]account.Account, 0)
	for rows.Next() {
		item, err := scanAccountBase(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	if err := r.hydrateAccounts(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *AccountRepository) ListPage(ctx context.Context, query appaccount.ListQuery) (appaccount.PageResult, error) {
	keyword := strings.TrimSpace(strings.ToLower(query.Keyword))
	whereClause := "WHERE a.tenant_id = ?"
	args := []any{query.TenantID}

	if keyword != "" {
		like := "%" + keyword + "%"
		whereClause += `
			AND (
				LOWER(a.username) LIKE ?
				OR LOWER(a.email) LIKE ?
				OR LOWER(a.display_name) LIKE ?
				OR LOWER(a.status) LIKE ?
				OR LOWER(r.name) LIKE ?
			)
		`
		args = append(args, like, like, like, like, like)
	}

	var total int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM accounts a LEFT JOIN roles r ON r.id = a.role_id AND r.tenant_id = a.tenant_id %s`, whereClause)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return appaccount.PageResult{}, fmt.Errorf("count accounts: %w", err)
	}

	offset := (query.Page - 1) * query.PageSize
	listQuery := fmt.Sprintf(`
		%s
		%s
		ORDER BY a.updated_at DESC, a.username ASC
		LIMIT ? OFFSET ?
	`, accountSelectColumns, whereClause)
	listArgs := append(append([]any{}, args...), query.PageSize, offset)
	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return appaccount.PageResult{}, fmt.Errorf("list accounts page: %w", err)
	}
	defer rows.Close()

	items := make([]account.Account, 0, query.PageSize)
	for rows.Next() {
		item, err := scanAccountBase(rows)
		if err != nil {
			return appaccount.PageResult{}, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return appaccount.PageResult{}, fmt.Errorf("iterate accounts page: %w", err)
	}
	if err := r.hydrateAccounts(ctx, items); err != nil {
		return appaccount.PageResult{}, err
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + query.PageSize - 1) / query.PageSize
	}

	return appaccount.PageResult{
		Items:      items,
		Page:       query.Page,
		PageSize:   query.PageSize,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

func (r *AccountRepository) GetByID(ctx context.Context, id string) (account.Account, error) {
	item, err := r.getOne(ctx, accountSelectColumns+`
		WHERE a.id = ?
	`, strings.TrimSpace(id))
	if err != nil {
		return account.Account{}, err
	}
	if err := r.hydrateAccount(ctx, &item); err != nil {
		return account.Account{}, err
	}
	return item, nil
}

func (r *AccountRepository) FindByLoginID(ctx context.Context, loginID string) (account.Account, error) {
	normalized := account.NormalizeLoginID(loginID)
	item, err := r.getOne(ctx, accountSelectColumns+`
		WHERE LOWER(a.username) = ? OR LOWER(a.email) = ?
		LIMIT 1
	`, normalized, normalized)
	if err != nil {
		return account.Account{}, err
	}
	if err := r.hydrateAccount(ctx, &item); err != nil {
		return account.Account{}, err
	}
	return item, nil
}

func (r *AccountRepository) Create(ctx context.Context, item account.Account) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO accounts (
			id, tenant_id, username, email, display_name, password_hash, role_id, status, version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		item.ID,
		item.TenantID,
		item.Username,
		nullString(item.Email),
		nullString(item.DisplayName),
		item.PasswordHash,
		nullString(item.RoleID),
		string(item.Status),
		item.Version,
		item.CreatedAt.UTC(),
		item.UpdatedAt.UTC(),
	)
	if err != nil {
		return mapMySQLError(err)
	}
	return nil
}

func (r *AccountRepository) Update(ctx context.Context, item account.Account) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE accounts
		SET tenant_id = ?, username = ?, email = ?, display_name = ?, password_hash = ?, role_id = ?, status = ?, version = ?, updated_at = ?
		WHERE id = ?
	`,
		item.TenantID,
		item.Username,
		nullString(item.Email),
		nullString(item.DisplayName),
		item.PasswordHash,
		nullString(item.RoleID),
		string(item.Status),
		item.Version,
		item.UpdatedAt.UTC(),
		item.ID,
	)
	if err != nil {
		return mapMySQLError(err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated rows: %w", err)
	}
	if affected == 0 {
		return account.ErrNotFound
	}
	return nil
}

func (r *AccountRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted rows: %w", err)
	}
	if affected == 0 {
		return account.ErrNotFound
	}
	return nil
}

func (r *AccountRepository) CountByRoleID(ctx context.Context, tenantID, roleID string) (int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM accounts
		WHERE tenant_id = ? AND role_id = ?
	`, tenantID, strings.TrimSpace(roleID)).Scan(&total); err != nil {
		return 0, fmt.Errorf("count accounts by role: %w", err)
	}
	return total, nil
}

func (r *AccountRepository) getOne(ctx context.Context, query string, args ...any) (account.Account, error) {
	row := r.db.QueryRowContext(ctx, query, args...)
	item, err := scanAccountBase(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return account.Account{}, account.ErrNotFound
		}
		return account.Account{}, err
	}
	return item, nil
}

func (r *AccountRepository) hydrateAccounts(ctx context.Context, items []account.Account) error {
	roleIDs := make([]string, 0, len(items))
	for _, item := range items {
		roleIDs = append(roleIDs, item.RoleID)
	}

	metaByID, err := loadRoleMetaByIDs(ctx, r.db, roleIDs)
	if err != nil {
		return err
	}

	for index := range items {
		if meta, ok := metaByID[items[index].RoleID]; ok {
			items[index].RoleName = meta.Name
			items[index].Roles = singleRoleList(meta.Name)
			items[index].Permissions = meta.Permissions
			continue
		}

		items[index].RoleName = strings.TrimSpace(items[index].RoleName)
		items[index].Roles = singleRoleList(items[index].RoleName)
		items[index].Permissions = nil
	}
	return nil
}

func (r *AccountRepository) hydrateAccount(ctx context.Context, item *account.Account) error {
	metaByID, err := loadRoleMetaByIDs(ctx, r.db, []string{item.RoleID})
	if err != nil {
		return err
	}
	if meta, ok := metaByID[item.RoleID]; ok {
		item.RoleName = meta.Name
		item.Roles = singleRoleList(meta.Name)
		item.Permissions = meta.Permissions
		return nil
	}

	item.RoleName = strings.TrimSpace(item.RoleName)
	item.Roles = singleRoleList(item.RoleName)
	item.Permissions = nil
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAccountBase(row scanner) (account.Account, error) {
	var (
		id           string
		tenantID     string
		username     string
		email        sql.NullString
		displayName  sql.NullString
		passwordHash string
		roleID       sql.NullString
		roleName     sql.NullString
		status       string
		version      int
		createdAt    sql.NullTime
		updatedAt    sql.NullTime
	)

	if err := row.Scan(
		&id,
		&tenantID,
		&username,
		&email,
		&displayName,
		&passwordHash,
		&roleID,
		&roleName,
		&status,
		&version,
		&createdAt,
		&updatedAt,
	); err != nil {
		return account.Account{}, fmt.Errorf("scan account: %w", err)
	}

	roleNameValue := strings.TrimSpace(roleName.String)
	return account.Account{
		ID:           id,
		TenantID:     tenantID,
		Username:     username,
		Email:        strings.TrimSpace(email.String),
		DisplayName:  strings.TrimSpace(displayName.String),
		PasswordHash: passwordHash,
		RoleID:       strings.TrimSpace(roleID.String),
		RoleName:     roleNameValue,
		Roles:        singleRoleList(roleNameValue),
		Status:       account.Status(status),
		Version:      version,
		CreatedAt:    createdAt.Time.UTC(),
		UpdatedAt:    updatedAt.Time.UTC(),
	}, nil
}

func singleRoleList(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return []string{trimmed}
}

func nullString(value string) sql.NullString {
	trimmed := strings.TrimSpace(value)
	return sql.NullString{String: trimmed, Valid: trimmed != ""}
}

func mapMySQLError(err error) error {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		message := strings.ToLower(mysqlErr.Message)
		switch {
		case strings.Contains(message, "uq_accounts_tenant_username"):
			return account.ErrDuplicateUsername
		case strings.Contains(message, "uq_accounts_tenant_email"):
			return account.ErrDuplicateEmail
		}
	}
	return err
}

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	appcredential "playground/internal/application/credential"
	domaincredential "playground/internal/domain/credential"
)

type CredentialRepository struct {
	db *sql.DB
}

func NewCredentialRepository(db *sql.DB) *CredentialRepository {
	return &CredentialRepository{db: db}
}

func (r *CredentialRepository) List(ctx context.Context, query appcredential.ListQuery) (appcredential.PageResult, error) {
	keyword := strings.TrimSpace(strings.ToLower(query.Keyword))
	whereClause := "WHERE tenant_id = ? AND owner_account_id = ?"
	args := []any{query.TenantID, query.OwnerAccountID}

	if keyword != "" {
		like := "%" + keyword + "%"
		whereClause += `
			AND (
				LOWER(title) LIKE ?
				OR LOWER(username) LIKE ?
				OR LOWER(website) LIKE ?
				OR LOWER(category) LIKE ?
			)
		`
		args = append(args, like, like, like, like)
	}

	var total int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM credential_records %s`, whereClause)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return appcredential.PageResult{}, fmt.Errorf("count credential records: %w", err)
	}

	offset := (query.Page - 1) * query.PageSize
	listQuery := fmt.Sprintf(`
		SELECT id, tenant_id, owner_account_id, title, username, website, category, notes, password_envelope, created_by, updated_by, created_at, updated_at
		FROM credential_records
		%s
		ORDER BY updated_at DESC, title ASC
		LIMIT ? OFFSET ?
	`, whereClause)
	listArgs := append(append([]any{}, args...), query.PageSize, offset)
	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return appcredential.PageResult{}, fmt.Errorf("list credential records: %w", err)
	}
	defer rows.Close()

	items := make([]domaincredential.Credential, 0, query.PageSize)
	for rows.Next() {
		item, err := scanCredential(rows)
		if err != nil {
			return appcredential.PageResult{}, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return appcredential.PageResult{}, fmt.Errorf("iterate credential records: %w", err)
	}

	return appcredential.PageResult{
		Items:      items,
		Page:       query.Page,
		PageSize:   query.PageSize,
		Total:      total,
		TotalPages: appcredential.TotalPages(total, query.PageSize),
	}, nil
}

func (r *CredentialRepository) GetByID(ctx context.Context, tenantID, ownerAccountID, id string) (domaincredential.Credential, error) {
	return r.getOne(ctx, `
		SELECT id, tenant_id, owner_account_id, title, username, website, category, notes, password_envelope, created_by, updated_by, created_at, updated_at
		FROM credential_records
		WHERE tenant_id = ? AND owner_account_id = ? AND id = ?
	`, tenantID, ownerAccountID, strings.TrimSpace(id))
}

func (r *CredentialRepository) Create(ctx context.Context, item domaincredential.Credential) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO credential_records (
			id, tenant_id, owner_account_id, title, username, website, category, notes, password_envelope, created_by, updated_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		item.ID,
		item.TenantID,
		item.OwnerAccountID,
		item.Title,
		item.Username,
		nullString(item.Website),
		nullString(item.Category),
		nullString(item.Notes),
		item.PasswordEnvelope,
		nullString(item.CreatedBy),
		nullString(item.UpdatedBy),
		item.CreatedAt.UTC(),
		item.UpdatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("create credential record: %w", err)
	}
	return nil
}

func (r *CredentialRepository) Update(ctx context.Context, item domaincredential.Credential) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE credential_records
		SET title = ?, username = ?, website = ?, category = ?, notes = ?, password_envelope = ?, updated_by = ?, updated_at = ?
		WHERE tenant_id = ? AND owner_account_id = ? AND id = ?
	`,
		item.Title,
		item.Username,
		nullString(item.Website),
		nullString(item.Category),
		nullString(item.Notes),
		item.PasswordEnvelope,
		nullString(item.UpdatedBy),
		item.UpdatedAt.UTC(),
		item.TenantID,
		item.OwnerAccountID,
		item.ID,
	)
	if err != nil {
		return fmt.Errorf("update credential record: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated rows: %w", err)
	}
	if affected == 0 {
		return domaincredential.ErrNotFound
	}
	return nil
}

func (r *CredentialRepository) Delete(ctx context.Context, tenantID, ownerAccountID, id string) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM credential_records
		WHERE tenant_id = ? AND owner_account_id = ? AND id = ?
	`, tenantID, ownerAccountID, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete credential record: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted rows: %w", err)
	}
	if affected == 0 {
		return domaincredential.ErrNotFound
	}
	return nil
}

func (r *CredentialRepository) getOne(ctx context.Context, query string, args ...any) (domaincredential.Credential, error) {
	row := r.db.QueryRowContext(ctx, query, args...)
	item, err := scanCredential(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domaincredential.Credential{}, domaincredential.ErrNotFound
		}
		return domaincredential.Credential{}, err
	}
	return item, nil
}

type credentialScanner interface {
	Scan(dest ...any) error
}

func scanCredential(row credentialScanner) (domaincredential.Credential, error) {
	var (
		item             domaincredential.Credential
		ownerAccountID   string
		website          sql.NullString
		category         sql.NullString
		notes            sql.NullString
		createdBy        sql.NullString
		updatedBy        sql.NullString
		createdAt        sql.NullTime
		updatedAt        sql.NullTime
		passwordEnvelope string
	)

	if err := row.Scan(
		&item.ID,
		&item.TenantID,
		&ownerAccountID,
		&item.Title,
		&item.Username,
		&website,
		&category,
		&notes,
		&passwordEnvelope,
		&createdBy,
		&updatedBy,
		&createdAt,
		&updatedAt,
	); err != nil {
		return domaincredential.Credential{}, fmt.Errorf("scan credential record: %w", err)
	}

	item.Website = strings.TrimSpace(website.String)
	item.Category = strings.TrimSpace(category.String)
	item.Notes = strings.TrimSpace(notes.String)
	item.OwnerAccountID = strings.TrimSpace(ownerAccountID)
	item.PasswordEnvelope = passwordEnvelope
	item.CreatedBy = strings.TrimSpace(createdBy.String)
	item.UpdatedBy = strings.TrimSpace(updatedBy.String)
	item.CreatedAt = createdAt.Time.UTC()
	item.UpdatedAt = updatedAt.Time.UTC()
	return item, nil
}

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	domainupload "playground/internal/domain/upload"
)

// UploadRepository 是 domain/upload.Repository 的 MySQL 实现。
// 只存元数据（谁上传、文件名、状态等），不存文件本身。
// 文件字节由 tusd 的 filestore 管理（storage/ 目录下的二进制文件）。
type UploadRepository struct {
	db *sql.DB
}

func NewUploadRepository(db *sql.DB) *UploadRepository {
	return &UploadRepository{db: db}
}

func (r *UploadRepository) Create(ctx context.Context, u domainupload.Upload) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO uploads
			(id, tenant_id, owner_account_id, filename, mime_type, size, status, storage_path, created_at)
		VALUES
			(?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, u.ID, u.TenantID, u.OwnerAccountID, u.Filename, u.MimeType, u.Size, u.Status, u.StoragePath, u.CreatedAt)
	if err != nil {
		return fmt.Errorf("create upload record: %w", err)
	}
	return nil
}

func (r *UploadRepository) GetByID(ctx context.Context, id string) (domainupload.Upload, error) {
	var u domainupload.Upload
	var completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, owner_account_id, filename, mime_type, size, status, storage_path, created_at, completed_at
		FROM uploads
		WHERE id = ?
	`, id).Scan(
		&u.ID, &u.TenantID, &u.OwnerAccountID,
		&u.Filename, &u.MimeType, &u.Size,
		&u.Status, &u.StoragePath, &u.CreatedAt, &completedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domainupload.Upload{}, domainupload.ErrNotFound
		}
		return domainupload.Upload{}, fmt.Errorf("get upload: %w", err)
	}

	if completedAt.Valid {
		t := completedAt.Time
		u.CompletedAt = &t
	}
	return u, nil
}

func (r *UploadRepository) MarkCompleted(ctx context.Context, id string, completedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE uploads SET status = ?, completed_at = ? WHERE id = ?
	`, domainupload.StatusCompleted, completedAt, id)
	if err != nil {
		return fmt.Errorf("mark upload completed: %w", err)
	}
	return nil
}

func (r *UploadRepository) ListByOwner(ctx context.Context, tenantID, ownerAccountID string) ([]domainupload.Upload, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, owner_account_id, filename, mime_type, size, status, storage_path, created_at, completed_at
		FROM uploads
		WHERE tenant_id = ? AND owner_account_id = ?
		ORDER BY created_at DESC
	`, tenantID, ownerAccountID)
	if err != nil {
		return nil, fmt.Errorf("list uploads: %w", err)
	}
	defer rows.Close()

	var items []domainupload.Upload
	for rows.Next() {
		var u domainupload.Upload
		var completedAt sql.NullTime
		if err := rows.Scan(
			&u.ID, &u.TenantID, &u.OwnerAccountID,
			&u.Filename, &u.MimeType, &u.Size,
			&u.Status, &u.StoragePath, &u.CreatedAt, &completedAt,
		); err != nil {
			return nil, fmt.Errorf("scan upload: %w", err)
		}
		if completedAt.Valid {
			t := completedAt.Time
			u.CompletedAt = &t
		}
		items = append(items, u)
	}
	return items, rows.Err()
}

func (r *UploadRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM uploads WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete upload: %w", err)
	}
	return nil
}

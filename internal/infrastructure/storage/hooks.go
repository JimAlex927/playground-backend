// Package storage 提供公共的 TUS hook 逻辑，供各存储后端（local、minio）复用。
//
// 设计原则：
//   - 存储后端（文件放哪）和业务 hook（谁上传、状态更新）是两个关注点，分开维护。
//   - 换存储后端时只改组装代码，hook 里的 MySQL 逻辑零改动。
package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	tusdhandler "github.com/tus/tusd/v2/pkg/handler"

	domainupload "playground/internal/domain/upload"
)

// HookRepository 是 hook 逻辑所需的最小持久化能力。
type HookRepository interface {
	Create(ctx context.Context, upload domainupload.Upload) error
	MarkCompleted(ctx context.Context, id string, completedAt time.Time) error
	Delete(ctx context.Context, id string) error
}

// HookCallbacks 是三个 TUS hook 的集合，直接赋值到 tusdhandler.Config 对应字段。
type HookCallbacks struct {
	PreUploadCreate    func(tusdhandler.HookEvent) (tusdhandler.HTTPResponse, tusdhandler.FileInfoChanges, error)
	PreFinishResponse  func(tusdhandler.HookEvent) (tusdhandler.HTTPResponse, error)
	PreUploadTerminate func(tusdhandler.HookEvent) (tusdhandler.HTTPResponse, error)
}

// BuildHooks 构建公共 TUS hook，供各存储后端复用。
//
// 关键设计细节：
//   - PreUploadCreateCallback 触发时 hook.Upload.ID 为空（tusd 尚未分配 ID）
//     因此由 hook 自行生成 ID，通过 FileInfoChanges.ID 返回给 tusd，确保 DB 和 tusd 用同一个 ID。
//   - StoragePath 属于存储后端私有信息（本地路径 or S3 key），不在公共 hook 中处理。
func BuildHooks(repo HookRepository) HookCallbacks {
	return HookCallbacks{
		PreUploadCreate: func(hook tusdhandler.HookEvent) (tusdhandler.HTTPResponse, tusdhandler.FileInfoChanges, error) {
			tenantID := hook.HTTPRequest.Header.Get("X-Auth-Tenant-Id")
			ownerAccountID := hook.HTTPRequest.Header.Get("X-Auth-User-Id")
			if tenantID == "" || ownerAccountID == "" {
				return tusdhandler.HTTPResponse{
					StatusCode: http.StatusUnauthorized,
					Body:       `{"error":"unauthorized"}`,
				}, tusdhandler.FileInfoChanges{}, fmt.Errorf("missing auth headers")
			}

			// 在 hook 里生成 ID，因为 hook 触发时 tusd 还没有分配 ID。
			// 通过 FileInfoChanges.ID 告知 tusd 使用这个 ID，DB 和 tusd 就用同一个 ID。
			id := newUploadID()

			// 只保留业务字段，丢弃客户端可能注入的任意 key，
			// 同时写入服务端可信的身份信息。
			metadata := tusdhandler.MetaData{
				"filename":       hook.Upload.MetaData["filename"],
				"filetype":       hook.Upload.MetaData["filetype"],
				"tenantId":       tenantID,
				"ownerAccountId": ownerAccountID,
			}

			if err := repo.Create(hook.Context, domainupload.Upload{
				ID:             id,
				TenantID:       tenantID,
				OwnerAccountID: ownerAccountID,
				Filename:       hook.Upload.MetaData["filename"],
				MimeType:       hook.Upload.MetaData["filetype"],
				Size:           hook.Upload.Size,
				Status:         domainupload.StatusUploading,
				CreatedAt:      time.Now().UTC(),
			}); err != nil {
				return tusdhandler.HTTPResponse{
					StatusCode: http.StatusInternalServerError,
					Body:       `{"error":"failed to create upload record"}`,
				}, tusdhandler.FileInfoChanges{}, err
			}

			return tusdhandler.HTTPResponse{}, tusdhandler.FileInfoChanges{
				ID:       id,
				MetaData: metadata,
			}, nil
		},

		PreFinishResponse: func(hook tusdhandler.HookEvent) (tusdhandler.HTTPResponse, error) {
			if err := repo.MarkCompleted(hook.Context, hook.Upload.ID, time.Now().UTC()); err != nil {
				return tusdhandler.HTTPResponse{}, fmt.Errorf("mark upload completed: %w", err)
			}
			return tusdhandler.HTTPResponse{}, nil
		},

		PreUploadTerminate: func(hook tusdhandler.HookEvent) (tusdhandler.HTTPResponse, error) {
			if err := repo.Delete(hook.Context, hook.Upload.ID); err != nil {
				return tusdhandler.HTTPResponse{}, fmt.Errorf("delete upload record: %w", err)
			}
			return tusdhandler.HTTPResponse{}, nil
		},
	}
}

func newUploadID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("upload_%d", time.Now().UnixNano())
	}
	return "upload_" + hex.EncodeToString(buf)
}

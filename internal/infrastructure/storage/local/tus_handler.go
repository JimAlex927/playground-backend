// Package local 提供基于本地文件系统的 TUS 上传存储实现。
package local

import (
	"fmt"
	"os"

	"github.com/tus/tusd/v2/pkg/filestore"
	tusdhandler "github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/memorylocker"

	infrastorage "playground/internal/infrastructure/storage"
)

// NewTusHandler 创建本地文件系统的 TUS Handler。
//
// 职责划分：
//   - 文件字节 I/O：filestore.FileStore（写到本地磁盘）
//   - 并发控制：memorylocker（防止同一 upload 并发写）
//   - 业务 hook（auth 校验 / MySQL 元数据）：infrastorage.BuildHooks（公共逻辑）
func NewTusHandler(dir string, repo infrastorage.HookRepository, maxSize int64) (*tusdhandler.Handler, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create upload dir %q: %w", dir, err)
	}

	store := filestore.FileStore{Path: dir}
	locker := memorylocker.New()

	composer := tusdhandler.NewStoreComposer()
	store.UseIn(composer)
	locker.UseIn(composer)

	hooks := infrastorage.BuildHooks(repo)

	config := tusdhandler.Config{
		BasePath:                   "/api/v1/tus/",
		StoreComposer:              composer,
		MaxSize:                    maxSize,
		DisableDownload:            true,
		RespectForwardedHeaders:    true,
		PreUploadCreateCallback:    hooks.PreUploadCreate,
		PreFinishResponseCallback:  hooks.PreFinishResponse,
		PreUploadTerminateCallback: hooks.PreUploadTerminate,
	}

	return tusdhandler.NewHandler(config)
}

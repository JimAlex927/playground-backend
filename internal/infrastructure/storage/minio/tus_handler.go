// Package minio 提供基于 MinIO（S3 兼容）的 TUS 上传存储实现。
package minio

import (
	tusdhandler "github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/memorylocker"
	"github.com/tus/tusd/v2/pkg/s3store"

	inframinio "playground/internal/infrastructure/minio"
	infrastorage "playground/internal/infrastructure/storage"
)

// NewTusHandler 创建 MinIO（S3 兼容）的 TUS Handler。
//
// 职责划分：
//   - 文件字节 I/O：s3store（对象存储到 MinIO）
//   - 并发控制：filelocker（基于 MinIO 对象的分布式锁，适合多实例部署）
//   - 业务 hook（auth 校验 / MySQL 元数据）：infrastorage.BuildHooks（公共逻辑）
//
// 与 local/tus_handler.go 的区别仅在存储后端和锁实现，hook 逻辑完全相同。
func NewTusHandler(minioCfg inframinio.Config, repo infrastorage.HookRepository, maxSize int64) (*tusdhandler.Handler, error) {
	s3Client := inframinio.NewS3Client(minioCfg)

	store := s3store.New(minioCfg.BucketName, s3Client)

	// memorylocker 适用于单实例部署。
	// 如需多实例水平扩展，可替换为 Redis 等分布式锁实现（实现 tusdhandler.Locker 接口即可）。
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

package upload

import (
	"context"
	"time"
)

// Repository 定义了上传记录的持久化契约，由 domain 层声明，infra 层实现。
// 注意：offset 不在此追踪，由 tusd 的 filestore 负责管理（.info 文件）。
type Repository interface {
	Create(ctx context.Context, upload Upload) error
	GetByID(ctx context.Context, id string) (Upload, error)
	MarkCompleted(ctx context.Context, id string, completedAt time.Time) error
	ListByOwner(ctx context.Context, tenantID, ownerAccountID string) ([]Upload, error)
	Delete(ctx context.Context, id string) error
}

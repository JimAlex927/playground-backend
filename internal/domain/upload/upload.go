package upload

import "time"

const (
	StatusUploading = "uploading"
	StatusCompleted = "completed"
)

// Upload 表示一次文件上传任务，是 upload 领域的核心实体。
// 它携带业务语义（谁上传、属于哪个租户），与底层存储实现无关。
type Upload struct {
	ID             string
	TenantID       string
	OwnerAccountID string
	Filename       string
	MimeType       string
	Size           int64
	Status         string
	StoragePath    string
	CreatedAt      time.Time
	CompletedAt    *time.Time
}

func (u Upload) IsComplete() bool {
	return u.Status == StatusCompleted
}

func (u Upload) BelongsTo(tenantID, ownerAccountID string) bool {
	return u.TenantID == tenantID && u.OwnerAccountID == ownerAccountID
}

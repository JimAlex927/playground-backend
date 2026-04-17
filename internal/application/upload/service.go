package upload

import (
	"context"

	domainupload "playground/internal/domain/upload"
)

// Repository 是 app 层对持久化能力的最小声明，与 domain.Repository 保持一致。
// app 层直接依赖这个接口，infra 通过依赖注入提供实现。
type Repository interface {
	GetByID(ctx context.Context, id string) (domainupload.Upload, error)
	ListByOwner(ctx context.Context, tenantID, ownerAccountID string) ([]domainupload.Upload, error)
	Delete(ctx context.Context, id string) error
}

// Service 是上传用例的 Application Service，负责流程编排。
// 它不知道 TUS 协议、不知道文件怎么存，只关心业务用例：谁能看、谁能删。
type Service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return Service{repo: repo}
}

// GetUpload 获取上传记录，校验归属权。
func (s Service) GetUpload(ctx context.Context, tenantID, ownerAccountID, id string) (domainupload.Upload, error) {
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domainupload.Upload{}, err
	}
	if !item.BelongsTo(tenantID, ownerAccountID) {
		return domainupload.Upload{}, domainupload.ErrForbidden
	}
	return item, nil
}

// ListUploads 列出当前用户的所有上传记录。
func (s Service) ListUploads(ctx context.Context, tenantID, ownerAccountID string) ([]domainupload.Upload, error) {
	return s.repo.ListByOwner(ctx, tenantID, ownerAccountID)
}

// DeleteUpload 删除上传记录（元数据），实际文件由 TUS 层在 terminate 时删除。
func (s Service) DeleteUpload(ctx context.Context, tenantID, ownerAccountID, id string) error {
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !item.BelongsTo(tenantID, ownerAccountID) {
		return domainupload.ErrForbidden
	}
	return s.repo.Delete(ctx, id)
}

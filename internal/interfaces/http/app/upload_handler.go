package app

import (
	"errors"
	"net/http"

	domainupload "playground/internal/domain/upload"
	"playground/internal/platform/httpx"
)

// handleUploads 处理 GET /api/v1/uploads —— 列出当前用户的上传记录。
func (h Handler) handleUploads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	principal, ok := principalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing authentication principal")
		return
	}

	items, err := h.uploads.ListUploads(r.Context(), principal.TenantID, principal.UserID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, toUploadPayload(item))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": payload})
}

// handleUploadByID 处理 GET/DELETE /api/v1/uploads/{id}。
func (h Handler) handleUploadByID(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing authentication principal")
		return
	}

	id := r.PathValue("id")
	switch r.Method {
	case http.MethodGet:
		item, err := h.uploads.GetUpload(r.Context(), principal.TenantID, principal.UserID, id)
		if err != nil {
			writeUploadError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toUploadPayload(item))

	case http.MethodDelete:
		if err := h.uploads.DeleteUpload(r.Context(), principal.TenantID, principal.UserID, id); err != nil {
			writeUploadError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "upload deleted"})

	default:
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func writeUploadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domainupload.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "upload not found")
	case errors.Is(err, domainupload.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "access denied")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func toUploadPayload(u domainupload.Upload) map[string]any {
	payload := map[string]any{
		"id":             u.ID,
		"tenantId":       u.TenantID,
		"ownerAccountId": u.OwnerAccountID,
		"filename":       u.Filename,
		"mimeType":       u.MimeType,
		"size":           u.Size,
		"status":         u.Status,
		"storagePath":    u.StoragePath,
		"createdAt":      u.CreatedAt,
	}
	if u.CompletedAt != nil {
		payload["completedAt"] = u.CompletedAt
	}
	return payload
}

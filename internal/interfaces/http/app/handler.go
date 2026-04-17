package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	appaccount "playground/internal/application/account"
	appauth "playground/internal/application/auth"
	appcredential "playground/internal/application/credential"
	apppermission "playground/internal/application/permission"
	approle "playground/internal/application/role"
	appupload "playground/internal/application/upload"
	domainaccount "playground/internal/domain/account"
	domainauth "playground/internal/domain/auth"
	domaincredential "playground/internal/domain/credential"
	domainpermission "playground/internal/domain/permission"
	domainrole "playground/internal/domain/role"
	"playground/internal/platform/httpx"
)

type Handler struct {
	accounts         appaccount.Service
	permissions      apppermission.Service
	roles            approle.Service
	credentials      appcredential.Service
	uploads          appupload.Service
	tokens           appauth.TokenManager
	allowDirectToken bool
	tusHandler       http.Handler // TUS 协议处理器，由 infra/storage/local 创建并注入
}

type principalContextKey struct{}

func NewHandler(
	accounts appaccount.Service,
	permissions apppermission.Service,
	roles approle.Service,
	credentials appcredential.Service,
	uploads appupload.Service,
	tusHandler http.Handler,
	tokens appauth.TokenManager,
	allowDirectToken bool,
) http.Handler {
	handler := Handler{
		accounts:         accounts,
		permissions:      permissions,
		roles:            roles,
		credentials:      credentials,
		uploads:          uploads,
		tusHandler:       tusHandler,
		tokens:           tokens,
		allowDirectToken: allowDirectToken,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handler.handleHealth)

	// Account 用例
	mux.Handle("/api/v1/accounts", handler.withPrincipal(http.HandlerFunc(handler.handleAccounts)))
	mux.Handle("/api/v1/accounts/{id}", handler.withPrincipal(http.HandlerFunc(handler.handleAccountByID)))
	mux.Handle("/api/v1/accounts/{id}/password", handler.withPrincipal(http.HandlerFunc(handler.handleChangePassword)))

	// Permission / Role 用例
	mux.Handle("/api/v1/permissions", handler.withPrincipal(http.HandlerFunc(handler.handlePermissions)))
	mux.Handle("/api/v1/permissions/{id}", handler.withPrincipal(http.HandlerFunc(handler.handlePermissionByID)))
	mux.Handle("/api/v1/roles", handler.withPrincipal(http.HandlerFunc(handler.handleRoles)))
	mux.Handle("/api/v1/roles/{id}", handler.withPrincipal(http.HandlerFunc(handler.handleRoleByID)))

	// Credential 用例
	mux.Handle("/api/v1/credentials", handler.withPrincipal(http.HandlerFunc(handler.handleCredentials)))
	mux.Handle("/api/v1/credentials/{id}", handler.withPrincipal(http.HandlerFunc(handler.handleCredentialByID)))
	mux.Handle("/api/v1/credentials/{id}/secret", handler.withPrincipal(http.HandlerFunc(handler.handleCredentialSecret)))

	// Upload 元数据用例（REST，查询/删除记录）
	mux.Handle("/api/v1/uploads", handler.withPrincipal(http.HandlerFunc(handler.handleUploads)))
	mux.Handle("/api/v1/uploads/{id}", handler.withPrincipal(http.HandlerFunc(handler.handleUploadByID)))

	// TUS 协议端点（分片上传协议：POST/PATCH/HEAD/DELETE）
	// 路径与 REST 路径分开，避免路由冲突。
	// withPrincipal 确保只有已认证用户才能访问 TUS 端点。
	mux.Handle("/api/v1/tus/", handler.withPrincipal(handler.tusHandler))

	return mux
}

func (h Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h Handler) handleAccounts(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing authentication principal")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !canReadAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsRead+" permission is required")
			return
		}

		page, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
		pageSize, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("pageSize")))
		result, err := h.accounts.ListAccountPage(r.Context(), appaccount.ListQuery{
			TenantID: principal.TenantID,
			Keyword:  r.URL.Query().Get("keyword"),
			Page:     page,
			PageSize: pageSize,
		})
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}

		payload := make([]map[string]any, 0, len(result.Items))
		for _, item := range result.Items {
			payload = append(payload, toAccountPayload(item))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"items":      payload,
			"page":       result.Page,
			"pageSize":   result.PageSize,
			"total":      result.Total,
			"totalPages": result.TotalPages,
			"keyword":    strings.TrimSpace(r.URL.Query().Get("keyword")),
		})
	case http.MethodPost:
		if !canWriteAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsWrite+" permission is required")
			return
		}

		var input appaccount.CreateAccountInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "request body must be valid JSON")
			return
		}

		input.TenantID = principal.TenantID
		item, err := h.accounts.CreateAccount(r.Context(), input)
		if err != nil {
			writeAccountError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusCreated, toAccountPayload(item))
	default:
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h Handler) handleAccountByID(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing authentication principal")
		return
	}

	id := r.PathValue("id")
	switch r.Method {
	case http.MethodGet:
		if !canReadAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsRead+" permission is required")
			return
		}

		item, err := h.accounts.GetAccount(r.Context(), id)
		if err != nil {
			writeAccountError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusOK, toAccountPayload(item))
	case http.MethodPut:
		if !canWriteAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsWrite+" permission is required")
			return
		}

		/**
		这里如果是put 会把用户的 version给更新 导致之前的旧的 token失效
		*/
		var input appaccount.UpdateAccountInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "request body must be valid JSON")
			return
		}

		item, err := h.accounts.UpdateAccount(r.Context(), id, input)
		if err != nil {
			writeAccountError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusOK, toAccountPayload(item))
	case http.MethodDelete:
		if !canWriteAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsWrite+" permission is required")
			return
		}

		if err := h.accounts.DeleteAccount(r.Context(), id); err != nil {
			writeAccountError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "account deleted"})
	default:
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h Handler) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	principal, ok := principalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing authentication principal")
		return
	}
	if !canWriteAccounts(principal) {
		httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsWrite+" permission is required")
		return
	}

	var input appaccount.ChangePasswordInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "request body must be valid JSON")
		return
	}

	item, err := h.accounts.ChangePassword(r.Context(), r.PathValue("id"), input)
	if err != nil {
		writeAccountError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, toAccountPayload(item))
}

func (h Handler) handleRoles(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing authentication principal")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !canReadAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsRead+" permission is required")
			return
		}

		items, err := h.roles.ListRoles(r.Context(), principal.TenantID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}

		payload := make([]map[string]any, 0, len(items))
		for _, item := range items {
			payload = append(payload, toRolePayload(item))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"items": payload,
		})
	case http.MethodPost:
		if !canWriteAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsWrite+" permission is required")
			return
		}

		var input approle.CreateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "request body must be valid JSON")
			return
		}

		input.TenantID = principal.TenantID
		item, err := h.roles.CreateRole(r.Context(), input)
		if err != nil {
			writeRoleError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, toRolePayload(item))
	default:
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h Handler) handlePermissions(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing authentication principal")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !canReadAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsRead+" permission is required")
			return
		}

		items, err := h.permissions.ListPermissions(r.Context(), principal.TenantID)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}

		payload := make([]map[string]any, 0, len(items))
		for _, item := range items {
			payload = append(payload, toPermissionPayload(item))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": payload})
	case http.MethodPost:
		if !canWriteAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsWrite+" permission is required")
			return
		}

		var input apppermission.CreateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "request body must be valid JSON")
			return
		}

		input.TenantID = principal.TenantID
		item, err := h.permissions.CreatePermission(r.Context(), input)
		if err != nil {
			writePermissionError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, toPermissionPayload(item))
	default:
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h Handler) handlePermissionByID(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing authentication principal")
		return
	}

	id := r.PathValue("id")
	switch r.Method {
	case http.MethodGet:
		if !canReadAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsRead+" permission is required")
			return
		}

		item, err := h.permissions.GetPermission(r.Context(), principal.TenantID, id)
		if err != nil {
			writePermissionError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toPermissionPayload(item))
	case http.MethodPut:
		if !canWriteAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsWrite+" permission is required")
			return
		}

		var input apppermission.UpdateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "request body must be valid JSON")
			return
		}

		item, err := h.permissions.UpdatePermission(r.Context(), principal.TenantID, id, input)
		if err != nil {
			writePermissionError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toPermissionPayload(item))
	case http.MethodDelete:
		if !canWriteAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsWrite+" permission is required")
			return
		}

		if err := h.permissions.DeletePermission(r.Context(), principal.TenantID, id); err != nil {
			writePermissionError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "permission deleted"})
	default:
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h Handler) handleRoleByID(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing authentication principal")
		return
	}

	id := r.PathValue("id")
	switch r.Method {
	case http.MethodGet:
		if !canReadAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsRead+" permission is required")
			return
		}

		item, err := h.roles.GetRole(r.Context(), principal.TenantID, id)
		if err != nil {
			writeRoleError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toRolePayload(item))
	case http.MethodPut:
		if !canWriteAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsWrite+" permission is required")
			return
		}

		var input approle.UpdateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "request body must be valid JSON")
			return
		}

		item, err := h.roles.UpdateRole(r.Context(), principal.TenantID, id, input)
		if err != nil {
			writeRoleError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toRolePayload(item))
	case http.MethodDelete:
		if !canWriteAccounts(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionAccountsWrite+" permission is required")
			return
		}

		if err := h.roles.DeleteRole(r.Context(), principal.TenantID, id); err != nil {
			writeRoleError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "role deleted"})
	default:
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h Handler) handleCredentials(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing authentication principal")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !canReadCredentials(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionCredentialsRead+" permission is required")
			return
		}

		page, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
		pageSize, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("pageSize")))
		result, err := h.credentials.ListCredentials(r.Context(), appcredential.ListQuery{
			TenantID:       principal.TenantID,
			OwnerAccountID: principal.UserID,
			Keyword:        r.URL.Query().Get("keyword"),
			Page:           page,
			PageSize:       pageSize,
		})
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}

		payload := make([]map[string]any, 0, len(result.Items))
		for _, item := range result.Items {
			payload = append(payload, toCredentialPayload(item))
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"items":      payload,
			"page":       result.Page,
			"pageSize":   result.PageSize,
			"total":      result.Total,
			"totalPages": result.TotalPages,
			"keyword":    strings.TrimSpace(r.URL.Query().Get("keyword")),
		})
	case http.MethodPost:
		if !canWriteCredentials(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionCredentialsWrite+" permission is required")
			return
		}

		var input appcredential.CreateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "request body must be valid JSON")
			return
		}

		input.TenantID = principal.TenantID
		input.OwnerAccountID = principal.UserID
		input.Actor = principal.Username
		item, err := h.credentials.CreateCredential(r.Context(), input)
		if err != nil {
			writeCredentialError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusCreated, toCredentialPayload(item))
	default:
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h Handler) handleCredentialByID(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing authentication principal")
		return
	}

	id := r.PathValue("id")
	switch r.Method {
	case http.MethodGet:
		if !canReadCredentials(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionCredentialsRead+" permission is required")
			return
		}

		item, err := h.credentials.GetCredential(r.Context(), principal.TenantID, principal.UserID, id)
		if err != nil {
			writeCredentialError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusOK, toCredentialPayload(item))
	case http.MethodPut:
		if !canWriteCredentials(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionCredentialsWrite+" permission is required")
			return
		}

		var input appcredential.UpdateInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "request body must be valid JSON")
			return
		}

		input.TenantID = principal.TenantID
		input.Actor = principal.Username
		item, err := h.credentials.UpdateCredential(r.Context(), principal.TenantID, principal.UserID, id, input)
		if err != nil {
			writeCredentialError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusOK, toCredentialPayload(item))
	case http.MethodDelete:
		if !canWriteCredentials(principal) {
			httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionCredentialsWrite+" permission is required")
			return
		}

		if err := h.credentials.DeleteCredential(r.Context(), principal.TenantID, principal.UserID, id); err != nil {
			writeCredentialError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "credential deleted"})
	default:
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h Handler) handleCredentialSecret(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	principal, ok := principalFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "missing authentication principal")
		return
	}

	if !canRevealCredentials(principal) {
		httpx.WriteError(w, http.StatusForbidden, domainrole.PermissionCredentialsReveal+" permission is required")
		return
	}

	password, err := h.credentials.RevealPassword(r.Context(), principal.TenantID, principal.UserID, r.PathValue("id"))
	if err != nil {
		writeCredentialError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"password": password,
	})
}

func (h Handler) withPrincipal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := h.resolvePrincipal(r)
		if err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "missing or invalid authentication")
			return
		}

		ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func principalFromContext(ctx context.Context) (domainauth.Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(domainauth.Principal)
	return principal, ok
}

func canReadAccounts(principal domainauth.Principal) bool {
	return principal.HasPermission(domainrole.PermissionAccountsRead)
}

func canWriteAccounts(principal domainauth.Principal) bool {
	return principal.HasPermission(domainrole.PermissionAccountsWrite)
}

func canReadCredentials(principal domainauth.Principal) bool {
	return principal.HasPermission(domainrole.PermissionCredentialsRead)
}

func canWriteCredentials(principal domainauth.Principal) bool {
	return principal.HasPermission(domainrole.PermissionCredentialsWrite)
}

func canRevealCredentials(principal domainauth.Principal) bool {
	return principal.HasPermission(domainrole.PermissionCredentialsReveal)
}

func (h Handler) resolvePrincipal(r *http.Request) (domainauth.Principal, error) {
	userID := strings.TrimSpace(r.Header.Get("X-Auth-User-Id"))
	if userID != "" {
		version, _ := strconv.Atoi(strings.TrimSpace(r.Header.Get("X-Auth-User-Version")))
		return domainauth.Principal{
			UserID:      userID,
			TenantID:    strings.TrimSpace(r.Header.Get("X-Auth-Tenant-Id")),
			Username:    strings.TrimSpace(r.Header.Get("X-Auth-Username")),
			Roles:       splitCommaHeader(r.Header.Get("X-Auth-Roles")),
			Permissions: splitCommaHeader(r.Header.Get("X-Auth-Permissions")),
			Version:     version,
		}, nil
	}

	if !h.allowDirectToken {
		return domainauth.Principal{}, domainaccount.ErrUnauthorized
	}

	token := bearerToken(r)
	if token == "" {
		return domainauth.Principal{}, domainaccount.ErrUnauthorized
	}

	claims, err := h.tokens.Parse(token)
	if err != nil {
		return domainauth.Principal{}, domainaccount.ErrUnauthorized
	}

	item, err := h.accounts.GetAccount(r.Context(), claims.UserID)
	if err != nil {
		return domainauth.Principal{}, domainaccount.ErrUnauthorized
	}
	if !item.CanLogin() {
		return domainauth.Principal{}, domainaccount.ErrForbidden
	}
	if item.Version != claims.Version || item.TenantID != claims.TenantID {
		return domainauth.Principal{}, domainaccount.ErrUnauthorized
	}

	return domainauth.Principal{
		UserID:      item.ID,
		TenantID:    item.TenantID,
		Username:    item.Username,
		Roles:       item.Roles,
		Permissions: item.Permissions,
		Version:     item.Version,
	}, nil
}

func writeAccountError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domainaccount.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "account not found")
	case errors.Is(err, domainaccount.ErrDuplicateUsername):
		httpx.WriteError(w, http.StatusConflict, "username already exists")
	case errors.Is(err, domainaccount.ErrDuplicateEmail):
		httpx.WriteError(w, http.StatusConflict, "email already exists")
	case errors.Is(err, domainrole.ErrNotFound):
		httpx.WriteError(w, http.StatusBadRequest, "role not found")
	default:
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	}
}

func toAccountPayload(item domainaccount.Account) map[string]any {
	return map[string]any{
		"id":          item.ID,
		"tenantId":    item.TenantID,
		"username":    item.Username,
		"email":       item.Email,
		"displayName": item.DisplayName,
		"roleId":      item.RoleID,
		"roleName":    item.RoleName,
		"roles":       item.Roles,
		"permissions": item.Permissions,
		"status":      item.Status,
		"version":     item.Version,
		"createdAt":   item.CreatedAt,
		"updatedAt":   item.UpdatedAt,
	}
}

func writeRoleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domainrole.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "role not found")
	case errors.Is(err, domainrole.ErrDuplicateName):
		httpx.WriteError(w, http.StatusConflict, "role name already exists")
	default:
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	}
}

func writePermissionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domainpermission.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "permission not found")
	case errors.Is(err, domainpermission.ErrDuplicateCode):
		httpx.WriteError(w, http.StatusConflict, "permission code already exists")
	case errors.Is(err, domainpermission.ErrSystemLocked):
		httpx.WriteError(w, http.StatusBadRequest, "system permission cannot be deleted")
	default:
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	}
}

func toRolePayload(item domainrole.Role) map[string]any {
	return map[string]any{
		"id":          item.ID,
		"tenantId":    item.TenantID,
		"name":        item.Name,
		"description": item.Description,
		"permissions": item.Permissions,
		"createdAt":   item.CreatedAt,
		"updatedAt":   item.UpdatedAt,
	}
}

func toPermissionPayload(item domainpermission.Permission) map[string]any {
	return map[string]any{
		"id":          item.ID,
		"tenantId":    item.TenantID,
		"code":        item.Code,
		"displayName": item.DisplayName,
		"description": item.Description,
		"system":      item.System,
		"createdAt":   item.CreatedAt,
		"updatedAt":   item.UpdatedAt,
	}
}

func writeCredentialError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domaincredential.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "credential not found")
	default:
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	}
}

func toCredentialPayload(item domaincredential.Credential) map[string]any {
	return map[string]any{
		"id":             item.ID,
		"tenantId":       item.TenantID,
		"ownerAccountId": item.OwnerAccountID,
		"title":          item.Title,
		"username":       item.Username,
		"website":        item.Website,
		"category":       item.Category,
		"notes":          item.Notes,
		"maskedPassword": maskPassword(),
		"createdBy":      item.CreatedBy,
		"updatedBy":      item.UpdatedBy,
		"createdAt":      item.CreatedAt,
		"updatedAt":      item.UpdatedAt,
	}
}

func maskPassword() string {
	return "••••••••"
}

func splitCommaHeader(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func bearerToken(r *http.Request) string {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return ""
	}

	parts := strings.Fields(authHeader)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return parts[1]
}

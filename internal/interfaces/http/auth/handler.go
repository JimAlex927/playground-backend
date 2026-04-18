package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	appauth "playground/internal/application/auth"
	domainaccount "playground/internal/domain/account"
	domainauth "playground/internal/domain/auth"
	"playground/internal/platform/httpx"
)

type Handler struct {
	service appauth.Service
}

func NewHandler(service appauth.Service) http.Handler {
	handler := Handler{service: service}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handler.handleHealth)
	mux.HandleFunc("POST /auth/login", handler.handleLogin)
	mux.HandleFunc("POST /auth/refresh", handler.handleRefresh)
	mux.HandleFunc("GET /auth/me", handler.handleMe)
	mux.HandleFunc("POST /auth/logout", handler.handleLogout)
	mux.HandleFunc("/internal/auth/verify", handler.handleVerify)
	return mux
}

type loginRequest struct {
	Email          string `json:"email"`
	Login          string `json:"login"`
	Password       string `json:"password"`
	PasswordBase64 string `json:"passwordBase64"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type logoutRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func (h Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "request body must be valid JSON")
		return
	}

	loginID := strings.TrimSpace(request.Login)
	if loginID == "" {
		loginID = strings.TrimSpace(request.Email)
	}

	password, err := decodePassword(request)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if loginID == "" || password == "" {
		httpx.WriteError(w, http.StatusBadRequest, "login/email and password are required")
		return
	}

	result, err := h.service.Login(r.Context(), appauth.LoginInput{
		LoginID:  loginID,
		Password: password,
	})
	if err != nil {
		writeAuthError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"accessToken":      result.AccessToken,
		"tokenType":        result.TokenType,
		"expiresAt":        result.ExpiresAt,
		"expiresIn":        result.ExpiresIn,
		"refreshToken":     result.RefreshToken,
		"refreshExpiresAt": result.RefreshExpiresAt,
		"refreshExpiresIn": result.RefreshExpiresIn,
		"user":             toUserPayload(result.User),
	})
}

func (h Handler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var request refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "request body must be valid JSON")
		return
	}

	result, err := h.service.Refresh(r.Context(), request.RefreshToken)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"accessToken":      result.AccessToken,
		"tokenType":        result.TokenType,
		"expiresAt":        result.ExpiresAt,
		"expiresIn":        result.ExpiresIn,
		"refreshToken":     result.RefreshToken,
		"refreshExpiresAt": result.RefreshExpiresAt,
		"refreshExpiresIn": result.RefreshExpiresIn,
		"user":             toUserPayload(result.User),
	})
}

func decodePassword(request loginRequest) (string, error) {
	if strings.TrimSpace(request.PasswordBase64) != "" {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(request.PasswordBase64))
		if err != nil {
			return "", errors.New("passwordBase64 must be a valid base64 string")
		}

		password := strings.TrimSpace(string(decoded))
		if password == "" {
			return "", errors.New("password is required")
		}
		return password, nil
	}

	password := strings.TrimSpace(request.Password)
	if password == "" {
		return "", errors.New("password is required")
	}
	return password, nil
}

func (h Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "missing bearer token")
		return
	}

	item, err := h.service.CurrentUser(r.Context(), token)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user": toUserPayload(item),
	})
}

func (h Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	var request logoutRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "request body must be valid JSON")
			return
		}
	}

	if err := h.service.Logout(r.Context(), request.RefreshToken); err != nil {
		writeAuthError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"message": "logout acknowledged, remove the bearer token and refresh token on the client side",
	})
}

func (h Handler) handleVerify(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "missing bearer token")
		return
	}

	principal, err := h.service.VerifyToken(r.Context(), token)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	applyForwardAuthHeaders(w.Header(), principal)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"user": map[string]any{
			"id":       principal.UserID,
			"tenantId": principal.TenantID,
			"username": principal.Username,
		},
	})
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domainaccount.ErrInvalidCredentials):
		httpx.WriteError(w, http.StatusUnauthorized, "username/email or password is incorrect")
	case errors.Is(err, domainaccount.ErrUnauthorized):
		httpx.WriteError(w, http.StatusUnauthorized, "token is invalid or expired")
	case errors.Is(err, domainaccount.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "account is disabled")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func applyForwardAuthHeaders(header http.Header, principal domainauth.Principal) {
	header.Set("X-Auth-User-Id", principal.UserID)
	header.Set("X-Auth-Tenant-Id", principal.TenantID)
	header.Set("X-Auth-Username", principal.Username)
	header.Set("X-Auth-Roles", strings.Join(principal.Roles, ","))
	header.Set("X-Auth-Permissions", strings.Join(principal.Permissions, ","))
	header.Set("X-Auth-User-Version", strconv.Itoa(principal.Version))
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

func toUserPayload(item domainaccount.Account) map[string]any {
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

package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainaccount "playground/internal/domain/account"
	domainauth "playground/internal/domain/auth"
)

type AccountRepository interface {
	FindByLoginID(ctx context.Context, loginID string) (domainaccount.Account, error)
	GetByID(ctx context.Context, id string) (domainaccount.Account, error)
}

type PasswordHasher interface {
	Compare(encodedHash, password string) error
}

type TokenManager interface {
	Issue(claims domainauth.Claims, ttl time.Duration) (string, time.Time, error)
	Parse(token string) (domainauth.Claims, error)
}

type Service struct {
	repo            AccountRepository
	hasher          PasswordHasher
	tokens          TokenManager
	principalCache  domainauth.PrincipalCache
	refreshSessions domainauth.RefreshSessionStore
	defaultTokenTTL time.Duration
	refreshTokenTTL time.Duration
}

type LoginInput struct {
	LoginID  string
	Password string
}

type LoginResult struct {
	AccessToken      string
	TokenType        string
	ExpiresAt        time.Time
	ExpiresIn        int64
	RefreshToken     string
	RefreshExpiresAt time.Time
	RefreshExpiresIn int64
	User             domainaccount.Account
}

func NewService(repo AccountRepository, hasher PasswordHasher, tokens TokenManager, principalCache domainauth.PrincipalCache, refreshSessions domainauth.RefreshSessionStore, defaultTokenTTL, refreshTokenTTL time.Duration) Service {
	return Service{
		repo:            repo,
		hasher:          hasher,
		tokens:          tokens,
		principalCache:  principalCache,
		refreshSessions: refreshSessions,
		defaultTokenTTL: defaultTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
	}
}

func (s Service) Login(ctx context.Context, input LoginInput) (LoginResult, error) {
	item, err := s.repo.FindByLoginID(ctx, input.LoginID)
	if err != nil {
		return LoginResult{}, domainaccount.ErrInvalidCredentials
	}

	if err := s.hasher.Compare(item.PasswordHash, input.Password); err != nil {
		return LoginResult{}, domainaccount.ErrInvalidCredentials
	}

	if !item.CanLogin() {
		return LoginResult{}, domainaccount.ErrForbidden
	}

	token, expiresAt, err := s.tokens.Issue(domainauth.Claims{
		UserID:   item.ID,
		TenantID: item.TenantID,
		Username: item.Username,
		Version:  item.Version,
	}, s.defaultTokenTTL)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue token: %w", err)
	}

	if s.principalCache != nil {
		_ = s.principalCache.Set(ctx, accountToPrincipal(item))
	}

	refreshToken, refreshExpiresAt, err := s.issueRefreshSession(ctx, item)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		AccessToken:      token,
		TokenType:        "Bearer",
		ExpiresAt:        expiresAt,
		ExpiresIn:        int64(time.Until(expiresAt).Seconds()),
		RefreshToken:     refreshToken,
		RefreshExpiresAt: refreshExpiresAt,
		RefreshExpiresIn: int64(time.Until(refreshExpiresAt).Seconds()),
		User:             item,
	}, nil
}

func (s Service) Refresh(ctx context.Context, refreshToken string) (LoginResult, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return LoginResult{}, domainaccount.ErrUnauthorized
	}

	if s.refreshSessions == nil {
		return LoginResult{}, fmt.Errorf("refresh session store is not configured")
	}

	session, ok, err := s.refreshSessions.Get(ctx, refreshToken)
	if err != nil {
		return LoginResult{}, err
	}
	if !ok {
		return LoginResult{}, domainaccount.ErrUnauthorized
	}

	item, err := s.repo.GetByID(ctx, session.UserID)
	if err != nil {
		_ = s.refreshSessions.Delete(ctx, refreshToken)
		return LoginResult{}, domainaccount.ErrUnauthorized
	}
	if !item.CanLogin() {
		_ = s.refreshSessions.Delete(ctx, refreshToken)
		return LoginResult{}, domainaccount.ErrForbidden
	}
	if item.Version != session.Version || item.TenantID != session.TenantID {
		_ = s.refreshSessions.Delete(ctx, refreshToken)
		return LoginResult{}, domainaccount.ErrUnauthorized
	}

	if s.principalCache != nil {
		_ = s.principalCache.Set(ctx, accountToPrincipal(item))
	}

	accessToken, expiresAt, err := s.tokens.Issue(domainauth.Claims{
		UserID:   item.ID,
		TenantID: item.TenantID,
		Username: item.Username,
		Version:  item.Version,
	}, s.defaultTokenTTL)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue token: %w", err)
	}

	if err := s.refreshSessions.Delete(ctx, refreshToken); err != nil {
		return LoginResult{}, err
	}

	newRefreshToken, refreshExpiresAt, err := s.issueRefreshSession(ctx, item)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		AccessToken:      accessToken,
		TokenType:        "Bearer",
		ExpiresAt:        expiresAt,
		ExpiresIn:        int64(time.Until(expiresAt).Seconds()),
		RefreshToken:     newRefreshToken,
		RefreshExpiresAt: refreshExpiresAt,
		RefreshExpiresIn: int64(time.Until(refreshExpiresAt).Seconds()),
		User:             item,
	}, nil
}

func (s Service) Logout(ctx context.Context, refreshToken string) error {
	if s.refreshSessions == nil {
		return nil
	}
	return s.refreshSessions.Delete(ctx, strings.TrimSpace(refreshToken))
}

func (s Service) VerifyToken(ctx context.Context, token string) (domainauth.Principal, error) {
	claims, err := s.tokens.Parse(strings.TrimSpace(token))
	if err != nil {
		return domainauth.Principal{}, domainaccount.ErrUnauthorized
	}

	if s.principalCache != nil {
		principal, ok, err := s.principalCache.Get(ctx, claims.TenantID, claims.UserID, claims.Version)
		if err == nil && ok {
			return principal, nil
		}
	}

	item, err := s.resolveAccountByClaims(ctx, claims)
	if err != nil {
		return domainauth.Principal{}, err
	}

	principal := accountToPrincipal(item)
	if s.principalCache != nil {
		_ = s.principalCache.Set(ctx, principal)
	}
	return principal, nil
}

func (s Service) CurrentUser(ctx context.Context, token string) (domainaccount.Account, error) {
	item, _, err := s.resolveAccountByToken(ctx, token)
	if err != nil {
		return domainaccount.Account{}, err
	}
	return item, nil
}

func (s Service) resolveAccountByToken(ctx context.Context, token string) (domainaccount.Account, domainauth.Claims, error) {
	claims, err := s.tokens.Parse(strings.TrimSpace(token))
	if err != nil {
		return domainaccount.Account{}, domainauth.Claims{}, domainaccount.ErrUnauthorized
	}

	item, err := s.resolveAccountByClaims(ctx, claims)
	if err != nil {
		return domainaccount.Account{}, domainauth.Claims{}, err
	}

	return item, claims, nil
}

func (s Service) resolveAccountByClaims(ctx context.Context, claims domainauth.Claims) (domainaccount.Account, error) {
	item, err := s.repo.GetByID(ctx, claims.UserID)
	if err != nil {
		return domainaccount.Account{}, domainaccount.ErrUnauthorized
	}

	if !item.CanLogin() {
		return domainaccount.Account{}, domainaccount.ErrForbidden
	}

	if item.Version != claims.Version || item.TenantID != claims.TenantID {
		return domainaccount.Account{}, domainaccount.ErrUnauthorized
	}

	return item, nil
}

func accountToPrincipal(item domainaccount.Account) domainauth.Principal {
	return domainauth.Principal{
		UserID:      item.ID,
		TenantID:    item.TenantID,
		Username:    item.Username,
		Roles:       item.Roles,
		Permissions: item.Permissions,
		Version:     item.Version,
	}
}

func (s Service) issueRefreshSession(ctx context.Context, item domainaccount.Account) (string, time.Time, error) {
	if s.refreshSessions == nil {
		return "", time.Time{}, fmt.Errorf("refresh session store is not configured")
	}

	return s.refreshSessions.Create(ctx, domainauth.RefreshSession{
		UserID:    item.ID,
		TenantID:  item.TenantID,
		Username:  item.Username,
		Version:   item.Version,
		CreatedAt: time.Now().UTC(),
	}, s.refreshTokenTTL)
}

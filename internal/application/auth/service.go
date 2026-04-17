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
	defaultTokenTTL time.Duration
}

type LoginInput struct {
	LoginID  string
	Password string
}

type LoginResult struct {
	AccessToken string
	TokenType   string
	ExpiresAt   time.Time
	ExpiresIn   int64
	User        domainaccount.Account
}

func NewService(repo AccountRepository, hasher PasswordHasher, tokens TokenManager, defaultTokenTTL time.Duration) Service {
	return Service{
		repo:            repo,
		hasher:          hasher,
		tokens:          tokens,
		defaultTokenTTL: defaultTokenTTL,
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
		UserID:      item.ID,
		TenantID:    item.TenantID,
		Username:    item.Username,
		Roles:       item.Roles,
		Permissions: item.Permissions,
		Version:     item.Version,
	}, s.defaultTokenTTL)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue token: %w", err)
	}

	return LoginResult{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
		ExpiresIn:   int64(time.Until(expiresAt).Seconds()),
		User:        item,
	}, nil
}

func (s Service) VerifyToken(ctx context.Context, token string) (domainauth.Principal, error) {
	item, _, err := s.resolveAccountByToken(ctx, token)
	if err != nil {
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

	item, err := s.repo.GetByID(ctx, claims.UserID)
	if err != nil {
		return domainaccount.Account{}, domainauth.Claims{}, domainaccount.ErrUnauthorized
	}

	if !item.CanLogin() {
		return domainaccount.Account{}, domainauth.Claims{}, domainaccount.ErrForbidden
	}

	if item.Version != claims.Version || item.TenantID != claims.TenantID {
		return domainaccount.Account{}, domainauth.Claims{}, domainaccount.ErrUnauthorized
	}

	return item, claims, nil
}

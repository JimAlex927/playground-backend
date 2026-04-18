package auth

import "time"

type Claims struct {
	UserID    string    `json:"sub"`
	TenantID  string    `json:"tenant_id"`
	Username  string    `json:"username"`
	Version   int       `json:"version"`
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
}

type Principal struct {
	UserID      string
	TenantID    string
	Username    string
	Roles       []string
	Permissions []string
	Version     int
}

func (p Principal) HasRole(role string) bool {
	for _, item := range p.Roles {
		if item == role {
			return true
		}
	}
	return false
}

func (p Principal) HasAnyRole(roles ...string) bool {
	for _, role := range roles {
		if p.HasRole(role) {
			return true
		}
	}
	return false
}

func (p Principal) HasPermission(permission string) bool {
	for _, item := range p.Permissions {
		if item == permission {
			return true
		}
	}
	return false
}

func (p Principal) HasAnyPermission(permissions ...string) bool {
	for _, permission := range permissions {
		if p.HasPermission(permission) {
			return true
		}
	}
	return false
}

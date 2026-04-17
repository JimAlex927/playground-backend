package permission

import "errors"

var (
	ErrNotFound      = errors.New("permission not found")
	ErrDuplicateCode = errors.New("permission code already exists")
	ErrSystemLocked  = errors.New("system permission cannot be deleted")
)

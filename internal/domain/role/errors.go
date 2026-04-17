package role

import "errors"

var (
	ErrNotFound      = errors.New("role not found")
	ErrDuplicateName = errors.New("role name already exists")
)

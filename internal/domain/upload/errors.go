package upload

import "errors"

var (
	ErrNotFound  = errors.New("upload not found")
	ErrForbidden = errors.New("access to upload is forbidden")
)

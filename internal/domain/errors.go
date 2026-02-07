package domain

import "errors"

var (
	ErrInvalidPath     = errors.New("invalid path")
	ErrMismatchedNames = errors.New("number of new names does not match number of files")
	ErrInvalidPattern  = errors.New("invalid pattern")
)

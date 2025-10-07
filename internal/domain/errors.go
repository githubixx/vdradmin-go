package domain

import "errors"

var (
	// ErrNotFound indicates a resource was not found
	ErrNotFound = errors.New("not found")

	// ErrInvalidInput indicates invalid input data
	ErrInvalidInput = errors.New("invalid input")

	// ErrUnauthorized indicates authentication failure
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates insufficient permissions
	ErrForbidden = errors.New("forbidden")

	// ErrConflict indicates a resource conflict
	ErrConflict = errors.New("conflict")

	// ErrConnection indicates a connection failure
	ErrConnection = errors.New("connection failed")

	// ErrTimeout indicates an operation timeout
	ErrTimeout = errors.New("timeout")

	// ErrInternal indicates an internal error
	ErrInternal = errors.New("internal error")
)

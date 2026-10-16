// Package errors defines the application error model shared by all services.
// Errors carry a stable machine-readable code, an HTTP status hint and an
// RFC 7807 representation. Domain and application layers return these errors;
// the HTTP adapter renders them.
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

type Code string

const (
	CodeValidation           Code = "VALIDATION_ERROR"
	CodeUnauthorized         Code = "UNAUTHORIZED"
	CodeInvalidCredentials   Code = "INVALID_CREDENTIALS"
	CodeTokenExpired         Code = "TOKEN_EXPIRED"
	CodeTokenReuse           Code = "TOKEN_REUSE_DETECTED"
	CodeAccountLocked        Code = "ACCOUNT_LOCKED"
	CodePasswordChangeNeeded Code = "PASSWORD_CHANGE_REQUIRED"
	CodePermissionDenied     Code = "PERMISSION_DENIED"
	CodeDelegationDenied     Code = "PERMISSION_DELEGATION_DENIED"
	CodeTenantMismatch       Code = "TENANT_MISMATCH"
	CodeNotFound             Code = "NOT_FOUND"
	CodeConflict             Code = "CONFLICT"
	CodeVersionConflict      Code = "VERSION_CONFLICT"
	CodeIdempotencyConflict  Code = "IDEMPOTENCY_CONFLICT"
	CodeRateLimited          Code = "RATE_LIMITED"
	CodeFieldNotAllowed      Code = "FIELD_NOT_ALLOWED"
	CodeInternal             Code = "INTERNAL_ERROR"
	CodeUnavailable          Code = "SERVICE_UNAVAILABLE"
)

// FieldError describes a single invalid input field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// AppError is the canonical application error.
type AppError struct {
	Code   Code         `json:"code"`
	Status int          `json:"status"`
	Title  string       `json:"title"`
	Detail string       `json:"detail"`
	Fields []FieldError `json:"errors,omitempty"`
	cause  error
}

func (e *AppError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Detail, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Detail)
}

func (e *AppError) Unwrap() error { return e.cause }

// WithCause attaches an underlying error (never rendered to clients).
func (e *AppError) WithCause(err error) *AppError {
	clone := *e
	clone.cause = err
	return &clone
}

// WithFields attaches per-field validation errors.
func (e *AppError) WithFields(fields ...FieldError) *AppError {
	clone := *e
	clone.Fields = append(clone.Fields, fields...)
	return &clone
}

func newErr(code Code, status int, title, detail string) *AppError {
	return &AppError{Code: code, Status: status, Title: title, Detail: detail}
}

func Validation(detail string) *AppError {
	return newErr(CodeValidation, http.StatusUnprocessableEntity, "Validation failed", detail)
}

func BadRequest(detail string) *AppError {
	return newErr(CodeValidation, http.StatusBadRequest, "Bad request", detail)
}

func Unauthorized(detail string) *AppError {
	return newErr(CodeUnauthorized, http.StatusUnauthorized, "Unauthorized", detail)
}

func InvalidCredentials() *AppError {
	return newErr(CodeInvalidCredentials, http.StatusUnauthorized, "Invalid credentials", "Email or password is incorrect.")
}

func TokenExpired() *AppError {
	return newErr(CodeTokenExpired, http.StatusUnauthorized, "Token expired", "The provided token has expired.")
}

func TokenReuse() *AppError {
	return newErr(CodeTokenReuse, http.StatusUnauthorized, "Token reuse detected", "This refresh token was already used. All sessions in this family were revoked.")
}

func AccountLocked(detail string) *AppError {
	return newErr(CodeAccountLocked, http.StatusForbidden, "Account locked", detail)
}

func PermissionDenied(detail string) *AppError {
	if detail == "" {
		detail = "You do not have permission to perform this action."
	}
	return newErr(CodePermissionDenied, http.StatusForbidden, "Permission denied", detail)
}

func DelegationDenied(detail string) *AppError {
	return newErr(CodeDelegationDenied, http.StatusForbidden, "Permission delegation denied", detail)
}

func NotFound(what string) *AppError {
	return newErr(CodeNotFound, http.StatusNotFound, "Not found", what+" not found.")
}

func Conflict(detail string) *AppError {
	return newErr(CodeConflict, http.StatusConflict, "Conflict", detail)
}

func VersionConflict() *AppError {
	return newErr(CodeVersionConflict, http.StatusConflict, "Version conflict", "The resource was modified concurrently. Reload and retry.")
}

func IdempotencyConflict() *AppError {
	return newErr(CodeIdempotencyConflict, http.StatusConflict, "Idempotency conflict", "This idempotency key was already used with a different request payload.")
}

func RateLimited() *AppError {
	return newErr(CodeRateLimited, http.StatusTooManyRequests, "Too many requests", "Rate limit exceeded. Retry later.")
}

func FieldNotAllowed(field string) *AppError {
	return newErr(CodeFieldNotAllowed, http.StatusForbidden, "Field not allowed",
		fmt.Sprintf("You do not have permission to use field %q.", field))
}

func Internal(err error) *AppError {
	return newErr(CodeInternal, http.StatusInternalServerError, "Internal error", "An unexpected error occurred.").WithCause(err)
}

func Unavailable(detail string) *AppError {
	return newErr(CodeUnavailable, http.StatusServiceUnavailable, "Service unavailable", detail)
}

// From converts any error into an *AppError (wrapping unknown errors as internal).
func From(err error) *AppError {
	var app *AppError
	if errors.As(err, &app) {
		return app
	}
	return Internal(err)
}

// Is reports whether err carries the given code.
func Is(err error, code Code) bool {
	var app *AppError
	if errors.As(err, &app) {
		return app.Code == code
	}
	return false
}

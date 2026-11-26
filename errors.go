package fluxsend

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	ErrAuthentication = errors.New("fluxsend: authentication failed")
	ErrAuthorization  = errors.New("fluxsend: authorization failed")
	ErrValidation     = errors.New("fluxsend: invalid request")
	ErrNotFound       = errors.New("fluxsend: resource not found")
	ErrConflict       = errors.New("fluxsend: resource conflict")
	ErrGone           = errors.New("fluxsend: resource no longer available")
	ErrRateLimit      = errors.New("fluxsend: rate limited")
	ErrServer         = errors.New("fluxsend: server error")
	ErrAPI            = errors.New("fluxsend: API error")
	ErrTransport      = errors.New("fluxsend: transport error")
)

// APIError describes a non-success HTTP response from FluxSend.
type APIError struct {
	Kind       error
	StatusCode int
	Message    string
	Data       json.RawMessage
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("fluxsend: HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("fluxsend: HTTP %d", e.StatusCode)
}

// Unwrap lets callers classify API errors with errors.Is.
func (e *APIError) Unwrap() error {
	return e.Kind
}

func apiError(statusCode int, message string, data json.RawMessage, header http.Header) *APIError {
	return &APIError{
		Kind:       errorKind(statusCode, message),
		StatusCode: statusCode,
		Message:    message,
		Data:       data,
		RetryAfter: parseRetryAfter(header.Get("Retry-After")),
	}
}

func errorKind(statusCode int, message string) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return ErrAuthentication
	case http.StatusForbidden:
		switch message {
		case "missing_api_key", "invalid_api_key", "invalid_api_key_binding":
			return ErrAuthentication
		default:
			return ErrAuthorization
		}
	case http.StatusBadRequest:
		return ErrValidation
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return ErrConflict
	case http.StatusGone:
		return ErrGone
	case http.StatusTooManyRequests:
		return ErrRateLimit
	}
	if statusCode >= http.StatusInternalServerError {
		return ErrServer
	}
	return ErrAPI
}

func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := time.ParseDuration(value + "s"); err == nil {
		return seconds
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		if delay := time.Until(retryAt); delay > 0 {
			return delay
		}
	}
	return 0
}

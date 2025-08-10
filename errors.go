package subzero

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Error types
var (
	ErrClientClosed     = errors.New("client is closed")
	ErrInvalidKey       = errors.New("key cannot be empty")
	ErrInvalidValue     = errors.New("value cannot be nil")
	ErrConnectionFailed = errors.New("connection failed")
	ErrTimeout          = errors.New("operation timed out")
	ErrServerError      = errors.New("server error")
	ErrInvalidConfig    = errors.New("invalid configuration")
	ErrKeyNotFound      = errors.New("key not found")
	ErrOperationFailed  = errors.New("operation failed")
)

type CacheError struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Operation string    `json:"operation"`
	Key       string    `json:"key,omitempty"`
	Cause     error     `json:"cause,omitempty"`
	Retryable bool      `json:"retryable"`
}

type ErrorCode int

const (
	ErrorCodeUnknown ErrorCode = iota
	ErrorCodeInvalidKey
	ErrorCodeInvalidValue
	ErrorCodeInvalidConfig
	ErrorCodeConnectionFailed
	ErrorCodeTimeout
	ErrorCodeServerError
	ErrorCodeKeyNotFound
	ErrorCodeOperationFailed
	ErrorCodeClientClosed
	ErrorCodeTooManyRetries
	ErrorCodeResourceExhausted
	ErrorCodePermissionDenied
)

func (e ErrorCode) String() string {
	switch e {
	case ErrorCodeInvalidKey:
		return "INVALID_KEY"
	case ErrorCodeInvalidValue:
		return "INVALID_VALUE"
	case ErrorCodeInvalidConfig:
		return "INVALID_CONFIG"
	case ErrorCodeConnectionFailed:
		return "CONNECTION_FAILED"
	case ErrorCodeTimeout:
		return "TIMEOUT"
	case ErrorCodeServerError:
		return "SERVER_ERROR"
	case ErrorCodeKeyNotFound:
		return "KEY_NOT_FOUND"
	case ErrorCodeOperationFailed:
		return "OPERATION_FAILED"
	case ErrorCodeClientClosed:
		return "CLIENT_CLOSED"
	case ErrorCodeTooManyRetries:
		return "TOO_MANY_RETRIES"
	case ErrorCodeResourceExhausted:
		return "RESOURCE_EXHAUSTED"
	case ErrorCodePermissionDenied:
		return "PERMISSION_DENIED"
	default:
		return "UNKNOWN"
	}
}

// Error Interface
func (e *CacheError) Error() string {
	if e.Key != "" {
		return fmt.Sprintf("%s: %s (key: %s, operation: %s)", e.Code.String(), e.Message, e.Key, e.Operation)
	}
	return fmt.Sprintf("%s: %s (operation: %s)", e.Code.String(), e.Message, e.Operation)
}

func (e *CacheError) Unwrap() error {
	return e.Cause
}

// IsRetryable returns whether this error should trigger a retry
func (e *CacheError) IsRetryable() bool {
	return e.Retryable
}

// IsTemporary returns whether this is a temporary error
func (e *CacheError) IsTemporary() bool {
	return e.Retryable
}

// NewCacheError creates a new CacheError
func NewCacheError(code ErrorCode, message, operation string) *CacheError {
	return &CacheError{
		Code:      code,
		Message:   message,
		Operation: operation,
		Retryable: isRetryableErrorCode(code),
	}
}

// NewCacheErrorWithKey creates a new CacheError with a key
func NewCacheErrorWithKey(code ErrorCode, message, operation, key string) *CacheError {
	return &CacheError{
		Code:      code,
		Message:   message,
		Operation: operation,
		Key:       key,
		Retryable: isRetryableErrorCode(code),
	}
}

// NewCacheErrorWithCause creates a new CacheError with an underlying cause
func NewCacheErrorWithCause(code ErrorCode, message, operation string, cause error) *CacheError {
	return &CacheError{
		Code:      code,
		Message:   message,
		Operation: operation,
		Cause:     cause,
		Retryable: isRetryableErrorCode(code),
	}
}

// isRetryableErrorCode determines if an error code should trigger retries
func isRetryableErrorCode(code ErrorCode) bool {
	switch code {
	case ErrorCodeConnectionFailed, ErrorCodeTimeout, ErrorCodeServerError, ErrorCodeResourceExhausted:
		return true
	case ErrorCodeInvalidKey, ErrorCodeInvalidValue, ErrorCodeInvalidConfig, ErrorCodeKeyNotFound,
		ErrorCodeClientClosed, ErrorCodePermissionDenied:
		return false
	default:
		return false
	}
}

// WrapGRPCError converts a gRPC error to a CacheError
func WrapGRPCError(err error, operation string) error {
	if err == nil {
		return nil
	}

	if grpcErr, ok := status.FromError(err); ok {
		switch grpcErr.Code() {
		case codes.InvalidArgument:
			return NewCacheErrorWithCause(ErrorCodeInvalidKey, grpcErr.Message(), operation, err)
		case codes.NotFound:
			return NewCacheErrorWithCause(ErrorCodeKeyNotFound, grpcErr.Message(), operation, err)
		case codes.Unavailable:
			return NewCacheErrorWithCause(ErrorCodeConnectionFailed, grpcErr.Message(), operation, err)
		case codes.DeadlineExceeded:
			return NewCacheErrorWithCause(ErrorCodeTimeout, grpcErr.Message(), operation, err)
		case codes.ResourceExhausted:
			return NewCacheErrorWithCause(ErrorCodeResourceExhausted, grpcErr.Message(), operation, err)
		case codes.PermissionDenied:
			return NewCacheErrorWithCause(ErrorCodePermissionDenied, grpcErr.Message(), operation, err)
		case codes.Internal, codes.Unknown:
			return NewCacheErrorWithCause(ErrorCodeServerError, grpcErr.Message(), operation, err)
		default:
			return NewCacheErrorWithCause(ErrorCodeOperationFailed, grpcErr.Message(), operation, err)
		}
	}

	// Handle non-gRPC errors
	errStr := err.Error()
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "no such host") {
		return NewCacheErrorWithCause(ErrorCodeConnectionFailed, errStr, operation, err)
	}

	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") {
		return NewCacheErrorWithCause(ErrorCodeTimeout, errStr, operation, err)
	}

	return NewCacheErrorWithCause(ErrorCodeOperationFailed, errStr, operation, err)
}

// WrapTCPError converts a TCP error to a CacheError
func WrapTCPError(err error, operation string) error {
	if err == nil {
		return nil
	}

	errStr := err.Error()

	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "EOF") {
		return NewCacheErrorWithCause(ErrorCodeConnectionFailed, errStr, operation, err)
	}

	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") {
		return NewCacheErrorWithCause(ErrorCodeTimeout, errStr, operation, err)
	}

	if strings.Contains(errStr, "not found") {
		return NewCacheErrorWithCause(ErrorCodeKeyNotFound, errStr, operation, err)
	}

	return NewCacheErrorWithCause(ErrorCodeOperationFailed, errStr, operation, err)
}

// IsCacheError checks if an error is a CacheError
func IsCacheError(err error) bool {
	_, ok := err.(*CacheError)
	return ok
}

// GetCacheError extracts a CacheError from an error chain
func GetCacheError(err error) *CacheError {
	var cacheErr *CacheError
	if errors.As(err, &cacheErr) {
		return cacheErr
	}
	return nil
}

// IsRetryableError checks if an error should trigger a retry
func IsRetryableError(err error) bool {
	if cacheErr := GetCacheError(err); cacheErr != nil {
		return cacheErr.IsRetryable()
	}
	return isRetryableError(err) // fallback to existing function
}

// IsConnectionError checks if an error is related to connection issues
func IsConnectionError(err error) bool {
	if cacheErr := GetCacheError(err); cacheErr != nil {
		return cacheErr.Code == ErrorCodeConnectionFailed
	}

	if grpcErr, ok := status.FromError(err); ok {
		return grpcErr.Code() == codes.Unavailable
	}

	errStr := err.Error()
	return strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "EOF")
}

// IsTimeoutError checks if an error is related to timeouts
func IsTimeoutError(err error) bool {
	if cacheErr := GetCacheError(err); cacheErr != nil {
		return cacheErr.Code == ErrorCodeTimeout
	}

	if grpcErr, ok := status.FromError(err); ok {
		return grpcErr.Code() == codes.DeadlineExceeded
	}

	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded")
}

// IsKeyNotFoundError checks if an error indicates a key was not found
func IsKeyNotFoundError(err error) bool {
	if cacheErr := GetCacheError(err); cacheErr != nil {
		return cacheErr.Code == ErrorCodeKeyNotFound
	}

	if grpcErr, ok := status.FromError(err); ok {
		return grpcErr.Code() == codes.NotFound
	}

	return false
}

// ErrorReporter provides structured error reporting capabilities
type ErrorReporter struct {
	OnError func(err *CacheError)
}

// NewErrorReporter creates a new error reporter
func NewErrorReporter(onError func(err *CacheError)) *ErrorReporter {
	return &ErrorReporter{
		OnError: onError,
	}
}

// ReportError reports an error if it's a CacheError
func (r *ErrorReporter) ReportError(err error) {
	if r.OnError == nil {
		return
	}

	if cacheErr := GetCacheError(err); cacheErr != nil {
		r.OnError(cacheErr)
	}
}

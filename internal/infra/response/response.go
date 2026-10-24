package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Standard error codes defined by DESIGN.md
const (
	CodeInvalidParam = "invalid_param"
	CodeUnauthorized = "unauthorized"
	CodeForbidden    = "forbidden"
	CodeNotFound     = "not_found"
	CodeConflict     = "conflict"
	CodeRateLimited  = "rate_limited"
	CodeInternal     = "internal"
)

// Response represents the unified JSON envelope for all API responses.
type Response struct {
	Data  interface{}   `json:"data"`
	Error *ErrorPayload `json:"error"`
}

// ErrorPayload represents the error details in a Response.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// AppError is an application-level business error with an HTTP status code.
type AppError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *AppError) Error() string {
	return "[" + e.Code + "] " + e.Message
}

// NewAppError creates a custom AppError.
func NewAppError(code, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

// Helper constructors for standard AppError
func ErrInvalidParam(msg string) *AppError {
	if msg == "" {
		msg = "Invalid parameters"
	}
	return NewAppError(CodeInvalidParam, msg, http.StatusBadRequest)
}

func ErrUnauthorized(msg string) *AppError {
	if msg == "" {
		msg = "Unauthorized"
	}
	return NewAppError(CodeUnauthorized, msg, http.StatusUnauthorized)
}

func ErrForbidden(msg string) *AppError {
	if msg == "" {
		msg = "Forbidden"
	}
	return NewAppError(CodeForbidden, msg, http.StatusForbidden)
}

func ErrNotFound(msg string) *AppError {
	if msg == "" {
		msg = "Resource not found"
	}
	return NewAppError(CodeNotFound, msg, http.StatusNotFound)
}

func ErrConflict(msg string) *AppError {
	if msg == "" {
		msg = "Resource conflict"
	}
	return NewAppError(CodeConflict, msg, http.StatusConflict)
}

func ErrRateLimited(msg string) *AppError {
	if msg == "" {
		msg = "Rate limit exceeded"
	}
	return NewAppError(CodeRateLimited, msg, http.StatusTooManyRequests)
}

func ErrInternal(msg string) *AppError {
	if msg == "" {
		msg = "Internal server error"
	}
	return NewAppError(CodeInternal, msg, http.StatusInternalServerError)
}

// Success writes a successful response with status code and data.
func Success(c *gin.Context, status int, data interface{}) {
	c.JSON(status, Response{
		Data:  data,
		Error: nil,
	})
}

// OK writes a 200 OK response with data.
func OK(c *gin.Context, data interface{}) {
	Success(c, http.StatusOK, data)
}

// Fail writes an error response, matching AppError or falling back to "internal".
func Fail(c *gin.Context, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.HTTPStatus, Response{
			Data: nil,
			Error: &ErrorPayload{
				Code:    appErr.Code,
				Message: appErr.Message,
			},
		})
		return
	}

	// Unrecognized error: fall back to internal
	c.JSON(http.StatusInternalServerError, Response{
		Data: nil,
		Error: &ErrorPayload{
			Code:    CodeInternal,
			Message: "Internal server error",
		},
	})
}

package middleware

import "github.com/labstack/echo/v5"

// StructuredHTTPError wraps *echo.HTTPError while preserving a structured JSON
// detail payload for the REST HTTPErrorHandler (Echo v5 HTTPError.Message is a
// plain string).
type StructuredHTTPError struct {
	*echo.HTTPError
	Detail map[string]any
}

// NewStructuredHTTPError returns a StructuredHTTPError that satisfies
// errors.As(..., &*echo.HTTPError) and echo.HTTPStatusCoder while retaining
// structured detail fields.
func NewStructuredHTTPError(code int, message string, detail map[string]any) *StructuredHTTPError {
	return &StructuredHTTPError{
		HTTPError: echo.NewHTTPError(code, message),
		Detail:    detail,
	}
}

// Unwrap returns the underlying *echo.HTTPError.
func (e *StructuredHTTPError) Unwrap() error {
	return e.HTTPError
}

// ErrorDetail returns the structured detail payload for HTTPErrorHandler.
func (e *StructuredHTTPError) ErrorDetail() any {
	return e.Detail
}

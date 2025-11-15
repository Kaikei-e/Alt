package errors

import "net/http"

// HTTPStatusCode maps error codes to HTTP status codes
func (e *AppError) HTTPStatusCode() int {
	switch e.Code {
	case ErrCodeValidation:
		return http.StatusBadRequest
	case ErrCodeRateLimit:
		return http.StatusTooManyRequests
	case ErrCodeExternalAPI:
		return http.StatusBadGateway
	case ErrCodeTimeout:
		return http.StatusGatewayTimeout
	case ErrCodeTLSCertificate:
		return http.StatusBadRequest
	case ErrCodeDatabase:
		return http.StatusInternalServerError
	case ErrCodeUnknown:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// HTTPErrorResponse represents the structure of error responses sent to clients
type HTTPErrorResponse struct {
	Error   string                 `json:"error"`
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// ToHTTPResponse converts an AppError to an HTTP error response
func (e *AppError) ToHTTPResponse() HTTPErrorResponse {
	return HTTPErrorResponse{
		Error:   "error",
		Code:    string(e.Code),
		Message: e.Message,
		Context: e.Context,
	}
}

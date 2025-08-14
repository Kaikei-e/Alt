package kratos

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"auth-service/app/domain"
	kratosclient "github.com/ory/kratos-client-go"
)

// transformKratosError transforms Kratos API errors to domain errors
func (a *KratosClientAdapter) transformKratosError(err error, httpResp *http.Response, operation string) error {
	a.logger.Error("transforming kratos error",
		"error", err,
		"error_type", fmt.Sprintf("%T", err),
		"operation", operation,
		"http_status", getHTTPStatus(httpResp))

	// Handle Kratos-specific errors
	if kratosErr, ok := err.(*kratosclient.GenericOpenAPIError); ok {
		return a.parseKratosGenericError(kratosErr, operation)
	}

	// Handle HTTP status codes
	if httpResp != nil {
		return a.parseHTTPStatusError(httpResp.StatusCode, operation, err)
	}

	// Fallback to generic error
	return domain.NewAuthError(domain.ErrCodeInternal, fmt.Sprintf("Kratos %s failed", operation), err)
}

// parseKratosGenericError parses Kratos GenericOpenAPIError
func (a *KratosClientAdapter) parseKratosGenericError(kratosErr *kratosclient.GenericOpenAPIError, operation string) error {
	body := kratosErr.Body()
	
	a.logger.Debug("parsing kratos generic error",
		"operation", operation,
		"body_length", len(body))

	// Try to parse as JSON
	var errorResp map[string]interface{}
	if jsonErr := json.Unmarshal(body, &errorResp); jsonErr == nil {
		return a.parseKratosErrorResponse(errorResp, operation)
	}

	// Try to parse as string
	bodyStr := string(body)
	return a.parseKratosErrorString(bodyStr, operation)
}

// parseKratosErrorResponse parses structured Kratos error response
func (a *KratosClientAdapter) parseKratosErrorResponse(errorResp map[string]interface{}, operation string) error {
	a.logger.Debug("parsing kratos error response",
		"operation", operation,
		"error_fields", getMapKeys(errorResp))

	// Check for UI errors (most detailed)
	if ui, ok := errorResp["ui"].(map[string]interface{}); ok {
		if err := a.parseUIErrors(ui, operation); err != nil {
			return err
		}
	}

	// Check for direct error fields
	if message, ok := errorResp["message"].(string); ok {
		return a.classifyErrorMessage(message, operation)
	}

	if reason, ok := errorResp["reason"].(string); ok {
		return a.classifyErrorMessage(reason, operation)
	}

	// Check for error object
	if errorObj, ok := errorResp["error"].(map[string]interface{}); ok {
		if message, ok := errorObj["message"].(string); ok {
			return a.classifyErrorMessage(message, operation)
		}
	}

	return domain.NewAuthError(domain.ErrCodeUnknown, "Unknown Kratos error", nil)
}

// parseUIErrors parses UI-level errors from Kratos
func (a *KratosClientAdapter) parseUIErrors(ui map[string]interface{}, operation string) error {
	// Parse messages array
	if messages, ok := ui["messages"].([]interface{}); ok {
		for _, msg := range messages {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				if text, ok := msgMap["text"].(string); ok {
					if err := a.classifyErrorMessage(text, operation); err != nil {
						// Return the first classified error we find
						return err
					}
				}
			}
		}
	}

	// Parse nodes for field-specific errors
	if nodes, ok := ui["nodes"].([]interface{}); ok {
		for _, node := range nodes {
			if nodeMap, ok := node.(map[string]interface{}); ok {
				if err := a.parseNodeErrors(nodeMap, operation); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// parseNodeErrors parses node-level errors
func (a *KratosClientAdapter) parseNodeErrors(node map[string]interface{}, operation string) error {
	if messages, ok := node["messages"].([]interface{}); ok {
		for _, msg := range messages {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				if text, ok := msgMap["text"].(string); ok {
					if err := a.classifyErrorMessage(text, operation); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// parseKratosErrorString parses string-based Kratos errors
func (a *KratosClientAdapter) parseKratosErrorString(errorStr, operation string) error {
	return a.classifyErrorMessage(errorStr, operation)
}

// parseHTTPStatusError parses HTTP status-based errors
func (a *KratosClientAdapter) parseHTTPStatusError(statusCode int, operation string, originalErr error) error {
	switch statusCode {
	case http.StatusBadRequest: // 400
		return domain.NewAuthError(domain.ErrCodeValidation, "Invalid request data", originalErr)
	case http.StatusUnauthorized: // 401
		return domain.NewAuthError(domain.ErrCodeUnauthorized, "Authentication failed", originalErr)
	case http.StatusForbidden: // 403
		return domain.NewAuthError(domain.ErrCodeForbidden, "Access denied", originalErr)
	case http.StatusNotFound: // 404
		return domain.NewAuthError(domain.ErrCodeNotFound, "Resource not found", originalErr)
	case http.StatusConflict: // 409
		return domain.NewAuthError(domain.ErrCodeUserExists, "User already exists", originalErr)
	case http.StatusGone: // 410
		return domain.NewAuthError(domain.ErrCodeFlowExpired, "Flow has expired", originalErr)
	case http.StatusUnprocessableEntity: // 422
		return domain.NewAuthError(domain.ErrCodeValidation, "Validation failed", originalErr)
	case http.StatusInternalServerError: // 500
		return domain.NewAuthError(domain.ErrCodeInternal, "Internal server error", originalErr)
	case http.StatusBadGateway: // 502
		return domain.NewAuthError(domain.ErrCodeServiceUnavailable, "Service temporarily unavailable", originalErr)
	case http.StatusServiceUnavailable: // 503
		return domain.NewAuthError(domain.ErrCodeServiceUnavailable, "Service temporarily unavailable", originalErr)
	case http.StatusGatewayTimeout: // 504
		return domain.NewAuthError(domain.ErrCodeTimeout, "Request timeout", originalErr)
	default:
		return domain.NewAuthError(domain.ErrCodeInternal, fmt.Sprintf("HTTP %d: %s failed", statusCode, operation), originalErr)
	}
}

// classifyErrorMessage classifies error messages into specific domain errors
func (a *KratosClientAdapter) classifyErrorMessage(message, operation string) error {
	messageLower := strings.ToLower(message)
	
	a.logger.Debug("classifying error message",
		"operation", operation,
		"message_snippet", truncateString(message, 100))

	// Email validation errors
	if containsAny(messageLower, []string{"property email is missing", "email is required", "missing email"}) {
		return domain.NewValidationError("email", nil, "Email is required for registration")
	}

	if containsAny(messageLower, []string{"invalid email", "email format", "email is not valid"}) {
		return domain.NewValidationError("email", nil, "Invalid email format")
	}

	// User existence errors
	if containsAny(messageLower, []string{"already exists", "already registered", "user exists", "duplicate"}) {
		return domain.NewAuthError(domain.ErrCodeUserExists, "User with this email already exists", nil)
	}

	// Password policy errors
	if containsAny(messageLower, []string{"password policy", "password requirement", "password too weak", "password must"}) {
		return domain.NewValidationError("password", nil, "Password does not meet security requirements")
	}

	// Password validation errors
	if containsAny(messageLower, []string{"password is required", "missing password", "property password is missing"}) {
		return domain.NewValidationError("password", nil, "Password is required")
	}

	// Flow expiration errors
	if containsAny(messageLower, []string{"flow expired", "flow has expired", "expired flow", "flow not found"}) {
		return domain.NewAuthError(domain.ErrCodeFlowExpired, "Authentication flow has expired. Please start over.", nil)
	}

	// Invalid credentials errors
	if containsAny(messageLower, []string{"invalid credentials", "wrong password", "authentication failed", "login failed"}) {
		return domain.NewAuthError(domain.ErrCodeUnauthorized, "Invalid email or password", nil)
	}

	// Session errors
	if containsAny(messageLower, []string{"session not found", "invalid session", "session expired"}) {
		return domain.NewAuthError(domain.ErrCodeSessionExpired, "Session has expired", nil)
	}

	// Traits validation errors
	if containsAny(messageLower, []string{"traits", "missing properties", "required properties"}) {
		return domain.NewValidationError("traits", nil, "Registration data is incomplete or invalid")
	}

	// Network/service errors
	if containsAny(messageLower, []string{"connection refused", "timeout", "network error", "service unavailable"}) {
		return domain.NewAuthError(domain.ErrCodeServiceUnavailable, "Authentication service is temporarily unavailable", nil)
	}

	// Generic validation errors
	if containsAny(messageLower, []string{"validation failed", "invalid input", "bad request"}) {
		return domain.NewAuthError(domain.ErrCodeValidation, "Request validation failed", nil)
	}

	// If we can't classify, create a generic error with the original message
	return domain.NewAuthError(domain.ErrCodeUnknown, fmt.Sprintf("Authentication error: %s", message), nil)
}

// Helper functions

// containsAny checks if the text contains any of the given substrings
func containsAny(text string, substrings []string) bool {
	for _, substring := range substrings {
		if strings.Contains(text, substring) {
			return true
		}
	}
	return false
}

// truncateString truncates a string to maxLength with ellipsis
func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength] + "..."
}
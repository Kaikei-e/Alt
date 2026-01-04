// Package errorhandler provides Connect-RPC error handling utilities.
// It ensures internal error details are never exposed to clients.
package errorhandler

import (
	"fmt"
	"log/slog"

	"connectrpc.com/connect"

	"alt/utils/errors"
)

// HandleConnectError converts internal errors to safe Connect-RPC errors.
// IMPORTANT: This function ensures internal error details are NEVER exposed to clients.
// All error messages are sanitized before being returned.
func HandleConnectError(logger *slog.Logger, err error, code connect.Code, operation string) *connect.Error {
	// Convert to AppContextError for consistent handling
	var enrichedErr *errors.AppContextError

	if appContextErr, ok := err.(*errors.AppContextError); ok {
		enrichedErr = errors.EnrichWithContext(
			appContextErr,
			"connect",
			"ConnectHandler",
			operation,
			nil,
		)
	} else if appErr, ok := err.(*errors.AppError); ok {
		enrichedErr = errors.NewAppContextError(
			string(appErr.Code),
			appErr.Message,
			"connect",
			"ConnectHandler",
			operation,
			appErr.Cause,
			nil,
		)
	} else {
		enrichedErr = errors.NewUnknownContextError(
			"internal server error",
			"connect",
			"ConnectHandler",
			operation,
			err,
			nil,
		)
	}

	// Log the full error details (internal only - never sent to client)
	logger.Error(
		"Connect-RPC Error",
		"error_id", enrichedErr.ErrorID,
		"error", enrichedErr.Error(),
		"code", enrichedErr.Code,
		"connect_code", code.String(),
		"operation", operation,
	)

	// Return safe error to client
	safeMessage := enrichedErr.SafeMessage()
	if enrichedErr.ErrorID != "" {
		safeMessage = fmt.Sprintf("%s (Error ID: %s)", safeMessage, enrichedErr.ErrorID)
	}

	return connect.NewError(code, fmt.Errorf("%s", safeMessage))
}

// HandleInternalError is a convenience wrapper for internal server errors
func HandleInternalError(logger *slog.Logger, err error, operation string) *connect.Error {
	return HandleConnectError(logger, err, connect.CodeInternal, operation)
}

// HandleValidationError is a convenience wrapper for validation errors
func HandleValidationError(logger *slog.Logger, message string, operation string) *connect.Error {
	validationErr := errors.NewValidationContextError(
		message,
		"connect",
		"ConnectHandler",
		operation,
		nil,
	)
	return HandleConnectError(logger, validationErr, connect.CodeInvalidArgument, operation)
}

// HandleNotFoundError is a convenience wrapper for not found errors
func HandleNotFoundError(logger *slog.Logger, message string, operation string) *connect.Error {
	notFoundErr := errors.NewAppContextError(
		"NOT_FOUND",
		message,
		"connect",
		"ConnectHandler",
		operation,
		nil,
		nil,
	)
	return HandleConnectError(logger, notFoundErr, connect.CodeNotFound, operation)
}

// HandleUnauthenticatedError is a convenience wrapper for authentication errors
func HandleUnauthenticatedError(logger *slog.Logger, err error, operation string) *connect.Error {
	return HandleConnectError(logger, err, connect.CodeUnauthenticated, operation)
}

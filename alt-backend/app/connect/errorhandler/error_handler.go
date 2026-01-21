// Package errorhandler provides Connect-RPC error handling utilities.
// It ensures internal error details are never exposed to clients.
package errorhandler

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"

	"alt/driver/search_indexer"
	"alt/utils/errors"
)

// HandleConnectError converts internal errors to safe Connect-RPC errors.
// IMPORTANT: This function ensures internal error details are NEVER exposed to clients.
// All error messages are sanitized before being returned.
func HandleConnectError(ctx context.Context, logger *slog.Logger, err error, code connect.Code, operation string) *connect.Error {
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
		// Check for specific driver errors before falling back to unknown error
		enrichedErr = classifyDriverError(err, operation)
	}

	// Log the full error details (internal only - never sent to client)
	logger.ErrorContext(ctx,
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
func HandleInternalError(ctx context.Context, logger *slog.Logger, err error, operation string) *connect.Error {
	return HandleConnectError(ctx, logger, err, connect.CodeInternal, operation)
}

// HandleValidationError is a convenience wrapper for validation errors
func HandleValidationError(ctx context.Context, logger *slog.Logger, message string, operation string) *connect.Error {
	validationErr := errors.NewValidationContextError(
		message,
		"connect",
		"ConnectHandler",
		operation,
		nil,
	)
	return HandleConnectError(ctx, logger, validationErr, connect.CodeInvalidArgument, operation)
}

// HandleNotFoundError is a convenience wrapper for not found errors
func HandleNotFoundError(ctx context.Context, logger *slog.Logger, message string, operation string) *connect.Error {
	notFoundErr := errors.NewAppContextError(
		"NOT_FOUND",
		message,
		"connect",
		"ConnectHandler",
		operation,
		nil,
		nil,
	)
	return HandleConnectError(ctx, logger, notFoundErr, connect.CodeNotFound, operation)
}

// HandleUnauthenticatedError is a convenience wrapper for authentication errors
func HandleUnauthenticatedError(ctx context.Context, logger *slog.Logger, err error, operation string) *connect.Error {
	return HandleConnectError(ctx, logger, err, connect.CodeUnauthenticated, operation)
}

// classifyDriverError checks for specific driver errors and returns appropriate AppContextError
func classifyDriverError(err error, operation string) *errors.AppContextError {
	// Check for search service unavailable error
	if stderrors.Is(err, search_indexer.ErrSearchServiceUnavailable) {
		return errors.NewExternalAPIContextError(
			"Search service is temporarily unavailable",
			"connect",
			"ConnectHandler",
			operation,
			err,
			map[string]interface{}{"service": "search-indexer"},
		)
	}

	// Check for search timeout error
	if stderrors.Is(err, search_indexer.ErrSearchTimeout) {
		return errors.NewTimeoutContextError(
			"Search request timed out",
			"connect",
			"ConnectHandler",
			operation,
			err,
			map[string]interface{}{"service": "search-indexer"},
		)
	}

	// Default to unknown error
	return errors.NewUnknownContextError(
		"internal server error",
		"connect",
		"ConnectHandler",
		operation,
		err,
		nil,
	)
}

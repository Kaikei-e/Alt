package logger

import (
	"alt/utils/errors"
	"context"
)

// SafeLogError safely logs an error with nil check for GlobalContext
func SafeLogError(ctx context.Context, operation string, err error) {
	if GlobalContext != nil {
		GlobalContext.LogError(ctx, operation, err)
	} else if err != nil {
		SafeError("operation failed", "operation", operation, "error", err.Error())
	}
}

// SafeLogInfo safely logs info with context using nil check for GlobalContext
func SafeLogInfo(ctx context.Context, msg string, args ...interface{}) {
	if GlobalContext != nil {
		GlobalContext.WithContext(ctx).Info(msg, args...)
	} else {
		SafeInfo(msg, args...)
	}
}

// SafeLogErrorWithAppContext safely logs AppContextError with enhanced details
func SafeLogErrorWithAppContext(ctx context.Context, operation string, appErr *errors.AppContextError) {
	if GlobalContext != nil {
		GlobalContext.LogError(ctx, operation, appErr)
	} else {
		SafeError("operation failed",
			"operation", operation,
			"error", appErr.Error(),
			"error_code", appErr.Code,
			"layer", appErr.Layer,
			"component", appErr.Component,
		)
	}
}

package bootstrap

import (
	"context"
	"fmt"
	"runtime/debug"

	"search-indexer/logger"
)

// logPanic emits a recovered panic to the logger without leaking the panic
// value at info/warn/error levels. The full value and stack trace are only
// visible at debug level, so production deployments running at LOG_LEVEL=info
// do not expose potentially sensitive wrapped data (tokens, URLs, SQL strings).
func logPanic(ctx context.Context, msg string, recovered any) {
	logger.Logger.ErrorContext(ctx, msg, "err_type", fmt.Sprintf("%T", recovered))
	logger.Logger.DebugContext(ctx, msg+" detail",
		"err", fmt.Sprintf("%v", recovered),
		"stack", string(debug.Stack()),
	)
}

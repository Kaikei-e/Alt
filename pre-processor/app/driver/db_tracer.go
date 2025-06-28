package driver

import (
	"context"
	"time"

	logger "pre-processor/utils/logger"

	"github.com/jackc/pgx/v5"
)

const (
	queryDurationThreshold = 100 * time.Millisecond
)

type QueryTracer struct {
}

func (t *QueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, "query start", time.Now())
}

func (t *QueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	queryStart, ok := ctx.Value("query start").(time.Time)
	if !ok {
		logger.Logger.Error("query start not found")
		return
	}

	duration := time.Since(queryStart)

	if duration > queryDurationThreshold {
		logger.Logger.Info("query executed", "duration", duration)
	}
}

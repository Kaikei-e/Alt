package driver

import (
	"context"
	"time"

	"log/slog"

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
		slog.Default().Error("query start not found")
		return
	}

	duration := time.Since(queryStart)

	if duration > queryDurationThreshold {
		slog.Default().Info("query executed", "duration", duration)
	}
}

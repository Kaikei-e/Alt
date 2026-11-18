//go:build contract

package contract

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"alt/domain"
	"alt/gen/proto/alt/reading/v1/readingv1connect"
	"alt/orchestrator/connect/v2/reading"
	"alt/orchestrator/usecase/record_reading_events_usecase"
)

type pactReadingAppendPort struct {
	seq atomic.Int64
}

func (p *pactReadingAppendPort) AppendKnowledgeEvent(_ context.Context, _ domain.KnowledgeEvent) (int64, error) {
	return p.seq.Add(1), nil
}

func mountReadingServiceProcedures(mux *http.ServeMux) {
	uc := record_reading_events_usecase.NewRecordReadingEventsUsecase(
		&pactReadingAppendPort{},
		func() time.Time { return time.Date(2026, 11, 18, 0, 5, 0, 0, time.UTC) },
	)
	handler := reading.NewHandler(uc, slog.Default())
	path, connectHandler := readingv1connect.NewReadingServiceHandler(handler)
	mux.Handle(path, withPactUserContext(connectHandler))
}

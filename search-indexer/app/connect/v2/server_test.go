package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"search-indexer/config"
	"search-indexer/domain"
	searchv2 "search-indexer/gen/proto/services/search/v2"
	"search-indexer/gen/proto/services/search/v2/searchv2connect"
	"search-indexer/logger"
	"search-indexer/port"
	"search-indexer/usecase"
)

func TestMain(m *testing.M) {
	logger.Init()
	m.Run()
}

type stubSearchEngine struct{}

func (stubSearchEngine) IndexDocuments(context.Context, []domain.SearchDocument) error {
	return nil
}
func (stubSearchEngine) DeleteDocuments(context.Context, []string) error { return nil }
func (stubSearchEngine) Search(context.Context, string, int) ([]domain.SearchDocument, error) {
	return nil, nil
}
func (stubSearchEngine) SearchWithFilters(context.Context, string, []string, int) ([]domain.SearchDocument, error) {
	return nil, nil
}
func (stubSearchEngine) SearchWithDateFilter(context.Context, string, *time.Time, *time.Time, int) ([]domain.SearchDocument, error) {
	return nil, nil
}
func (stubSearchEngine) EnsureIndex(context.Context) error { return nil }
func (stubSearchEngine) SearchByUserID(context.Context, string, string, int) ([]domain.SearchDocument, error) {
	return nil, nil
}
func (stubSearchEngine) SearchByUserIDWithPagination(context.Context, string, string, int64, int64) ([]domain.SearchDocument, int64, error) {
	return nil, 0, nil
}
func (stubSearchEngine) RegisterSynonyms(context.Context, map[string][]string) error { return nil }
func (stubSearchEngine) PruneTaskHistory(context.Context, time.Duration) error       { return nil }

var _ port.SearchEngine = stubSearchEngine{}

type stubRecapSearchEngine struct{}

func (stubRecapSearchEngine) EnsureRecapIndex(context.Context) error { return nil }
func (stubRecapSearchEngine) IndexRecapDocuments(context.Context, []domain.RecapDocument) error {
	return nil
}
func (stubRecapSearchEngine) SearchRecaps(context.Context, string, int) ([]domain.RecapDocument, int64, error) {
	return nil, 0, nil
}

var _ port.RecapSearchEngine = stubRecapSearchEngine{}

// TestCreateConnectServer_OtelInterceptor_RecordsSpan asserts that the Connect-RPC
// interceptor stack records a server-side span for every SearchService call.
// Without otelconnect wired, search-indexer Connect-RPC procedures stay invisible
// in otel_traces — only the raw HTTP /v1/search REST endpoint is captured.
func TestCreateConnectServer_OtelInterceptor_RecordsSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	original := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(original)

	uc := usecase.NewSearchByUserUsecase(stubSearchEngine{})
	rc := usecase.NewSearchRecapsUsecase(stubRecapSearchEngine{})

	handler := CreateConnectServer(uc, rc, config.RateLimitConfig{
		RequestsPerSecond: 100,
		Burst:             100,
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	client := searchv2connect.NewSearchServiceClient(http.DefaultClient, srv.URL)

	_, err := client.SearchArticles(context.Background(), connect.NewRequest(&searchv2.SearchArticlesRequest{
		Query:  "probe",
		UserId: "user-1",
		Limit:  1,
	}))
	if err != nil {
		t.Fatalf("SearchArticles returned error: %v", err)
	}

	spans := recorder.Ended()
	if len(spans) == 0 {
		t.Fatalf("expected at least one server-side Connect-RPC span, got none — otelconnect interceptor likely not wired")
	}

	var sawProcedureSpan bool
	for _, s := range spans {
		name := s.Name()
		if strings.Contains(name, "SearchArticles") || strings.Contains(name, "SearchService") {
			sawProcedureSpan = true
			break
		}
	}
	if !sawProcedureSpan {
		names := make([]string, 0, len(spans))
		for _, s := range spans {
			names = append(names, s.Name())
		}
		t.Fatalf("expected a span naming the SearchArticles procedure, got: %v", names)
	}
}

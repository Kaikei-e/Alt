package global_search_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"errors"
	"sort"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestGlobalSearchUsecase_EmitsPerSectionSpans is the observability anchor
// for Phase A: when global search runs, each of the three sections must emit
// its own span with duration / degraded attributes. Otherwise the otel_traces
// view stays flat and we cannot tell which section dominates Connect-RPC
// latency.
func TestGlobalSearchUsecase_EmitsPerSectionSpans(t *testing.T) {
	logger.InitLogger()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	uc := NewGlobalSearchUsecase(
		&mockArticleSearch{result: &domain.ArticleSearchSection{Hits: []domain.GlobalArticleHit{{ID: "a"}}}},
		&mockRecapSearch{err: errors.New("recap down")},
		&mockTagSearch{result: &domain.TagSearchSection{Hits: []domain.GlobalTagHit{{TagName: "x"}}}},
		tp.Tracer(TracerName),
	)

	if _, err := uc.Execute(userCtx(), "probe", 5, 3, 10); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	spans := recorder.Ended()
	names := make([]string, 0, len(spans))
	for _, s := range spans {
		names = append(names, s.Name())
	}
	sort.Strings(names)

	want := []string{
		"global_search.section.articles",
		"global_search.section.recaps",
		"global_search.section.tags",
	}
	if len(spans) != 3 {
		t.Fatalf("expected 3 section spans, got %d: %v", len(spans), names)
	}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("span[%d] = %q, want %q (all: %v)", i, names[i], n, names)
		}
	}

	// Confirm degraded recap section is marked as such.
	for _, s := range spans {
		if s.Name() != "global_search.section.recaps" {
			continue
		}
		var sawDegraded bool
		for _, attr := range s.Attributes() {
			if string(attr.Key) == "degraded" && attr.Value.AsBool() {
				sawDegraded = true
			}
		}
		if !sawDegraded {
			t.Errorf("recap span missing degraded=true attribute, attrs: %v", s.Attributes())
		}
	}
}

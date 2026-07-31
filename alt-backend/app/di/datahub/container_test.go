package datahub

import (
	"reflect"
	"testing"
)

func fieldNames(t *testing.T, v any) map[string]bool {
	t.Helper()
	rt := reflect.TypeOf(v)
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		t.Fatalf("expected a struct, got %s", rt.Kind())
	}
	names := make(map[string]bool, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		names[rt.Field(i).Name] = true
	}
	return names
}

// This is cmd/datahub's half of di/container_split_test.go's table, and it
// lives here rather than there because this root is a package of its own since
// ADR-000954 Wave 3 batch 6. It imports alt/di for the shared wiring-state
// loggers, so a test inside alt/di cannot reach it without an import cycle —
// and the reason for the split is exactly that: alt_db must be linked into
// this binary and no other, which is a property of packages rather than of
// fields.
//
// The same rule as the other two roots applies to what it must not build. A
// present-but-nil field is the ADR-000928 shape; an absent field is a compile
// error at the first reach.
func TestDataHubComponents_OmitsWhatItsBinaryDoesNotBuild(t *testing.T) {
	absent := []string{
		// data-hub serves DataHubService only: no crawling, no search,
		// no image pipeline, no admin surface.
		"SearchIndexerDriver", "RobotsTxtGateway", "ImageProxyUsecase",
		"AdminMonitor", "RagConnectClient", "FetchArticleGateway",
		"KnowledgeBackfillUsecase", "MetricsUsecase",
	}
	present := []string{
		"AltDBRepository", "KratosClient", "EventPublisher",
		"InternalArticleGateway", "RecapArticlesUsecase",
		"FetchRecentArticlesUsecase", "CreateSummaryVersionUsecase",
		"CreateTagSetVersionUsecase", "SovereignClient",
		"FetchTagCloudUsecase", "FetchArticlesByTagUsecase",
		// ADR-000954 Wave 3 batch 6. Asserted positively rather than left
		// implicit: these two are what alt-backend gave up its pool for, so a
		// root that stopped building them would take the Tag Trail and the
		// recall rail's fallback down with no other implementation anywhere.
		"TagTrailGateway", "ArticleRefGateway",
	}

	names := fieldNames(t, DataHubComponents{})
	for _, f := range absent {
		if names[f] {
			t.Errorf("datahub components must not carry field %s", f)
		}
	}
	for _, f := range present {
		if !names[f] {
			t.Errorf("datahub components must carry field %s", f)
		}
	}
}

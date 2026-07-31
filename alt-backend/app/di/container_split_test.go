package di

import (
	"reflect"
	"testing"
	"time"

	"alt/config"
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

// The three composition roots exist so that "this binary does not build X" is
// enforced by the compiler rather than by a nil field nobody checks. A field
// that is present but nil is exactly the ADR-000928 shape: a handler reaching
// for it cannot tell "DI forgot to wire this" from "deliberately disabled".
// Absent field, absent surface, compile error — that is the whole point of
// splitting the container, so it is pinned here.
func TestComponentStructs_OmitWhatTheirBinaryDoesNotBuild(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		absent  []string
		present []string
	}{
		{
			name:  "backend",
			value: ApplicationComponents{},
			absent: []string{
				// Moved to cmd/datahub with DataHubService (ADR-000954 D6/D7).
				"KratosClient", "EventPublisher", "RecapArticlesUsecase",
				"FetchRecentArticlesUsecase", "CreateTagSetVersionUsecase",
				// Never read by any handler (plan R11): copying them into three
				// roots would have tripled the dead wiring.
				"ConfigPort", "RateLimiterPort", "ErrorHandlerPort",
				"AppendKnowledgeEventUsecase",
			},
			present: []string{
				"AltDBRepository", "SovereignClient", "AdminMonitor",
				// RecallRail reads articles through the same gateway
				// DataHubService uses, so backend keeps it even though the
				// service-to-service RPC surface moved away.
				"InternalArticleGateway", "RecallRailUsecase",
				"CreateSummaryVersionUsecase", "ImageProxyUsecase", "CSRFTokenUsecase",
			},
		},
		{
			name:  "harvester",
			value: HarvesterComponents{},
			absent: []string{
				// None of the eight scheduled jobs search, publish events, or
				// answer HTTP, so none of this may be constructed here.
				"SearchIndexerDriver", "MQHubClient", "EventPublisher", "KratosClient",
				"CSRFTokenUsecase", "RagConnectClient", "AdminMonitor",
				"InternalArticleGateway", "RecapArticlesUsecase",
			},
			present: []string{
				"AltDBRepository", "ScrapingDomainUsecase", "RagIntegration",
				"SovereignClient", "ImageProxyUsecase", "FetchArticleGateway",
				"FetchTagCloudUsecase",
			},
		},
		{
			name:  "datahub",
			value: DataHubComponents{},
			absent: []string{
				// data-hub serves DataHubService only: no crawling, no search,
				// no image pipeline, no admin surface.
				"SearchIndexerDriver", "RobotsTxtGateway", "ImageProxyUsecase",
				"AdminMonitor", "RagConnectClient", "FetchArticleGateway",
				"KnowledgeBackfillUsecase", "MetricsUsecase",
			},
			present: []string{
				"AltDBRepository", "KratosClient", "EventPublisher",
				"InternalArticleGateway", "RecapArticlesUsecase",
				"FetchRecentArticlesUsecase", "CreateSummaryVersionUsecase",
				"CreateTagSetVersionUsecase", "SovereignClient",
				"FetchTagCloudUsecase", "FetchArticlesByTagUsecase",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names := fieldNames(t, tt.value)
			for _, f := range tt.absent {
				if names[f] {
					t.Errorf("%s components must not carry field %s", tt.name, f)
				}
			}
			for _, f := range tt.present {
				if !names[f] {
					t.Errorf("%s components must carry field %s", tt.name, f)
				}
			}
		})
	}
}

func splitTestConfig() *config.Config {
	return &config.Config{
		AppEnv: "development",
		Server: config.ServerConfig{
			Port: 9000, ConnectPort: 9101,
			ReadTimeout: 300 * time.Second, WriteTimeout: 300 * time.Second, IdleTimeout: 120 * time.Second,
		},
		RateLimit:     config.RateLimitConfig{ExternalAPIInterval: 10 * time.Second, ExternalAPIBurst: 3, FeedFetchLimit: 100},
		SearchIndexer: config.SearchIndexerConfig{ConnectURL: "http://search-indexer:9301"},
		MQHub:         config.MQHubConfig{Enabled: false, ConnectURL: "http://mq-hub:9500"},
	}
}

// newFeedModule used to return DeleteFeedLinkUsecase: nil and rely on the one
// composition root to patch it afterwards. With three roots, forgetting the
// patch in one of them still compiles and only fails as a nil dereference in
// the RSS DeleteFeedLink handler, so the dependency is passed in instead.
func TestNewFeedModule_WiresDeleteFeedLinkUsecase(t *testing.T) {
	infra := newInfraModule(nil, splitTestConfig())
	sub := newSubscriptionModule(infra)

	feed := newFeedModule(infra, sub)

	if feed.DeleteFeedLinkUsecase == nil {
		t.Fatal("newFeedModule must wire DeleteFeedLinkUsecase itself, not leave it for the composition root")
	}
	if feed.DeleteFeedLinkUsecase != sub.DeleteFeedLinkUsecase {
		t.Error("DeleteFeedLinkUsecase must be the subscription module's instance")
	}
}

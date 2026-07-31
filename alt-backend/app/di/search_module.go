package di

import (
	"alt/orchestrator/gateway/feed_url_link_gateway"
	"alt/orchestrator/gateway/global_search_gateway"
	"alt/orchestrator/usecase/global_search_usecase"

	"go.opentelemetry.io/otel"
)

// SearchModule holds all global-search-domain components.
type SearchModule struct {
	GlobalSearchUsecase *global_search_usecase.GlobalSearchUsecase
}

// newSearchModule creates the SearchModule and wires all global search components.
func newSearchModule(infra *InfraModule) *SearchModule {
	// The article-to-feed resolution moved to alt-data-hub in ADR-000954
	// Wave 3 batch 3 (catalog §2.H W3-H11) and the tag prefix search in batch
	// 4 (§2.J W3-J4), so this module no longer opens anything of its own.
	urlGW := feed_url_link_gateway.NewFeedURLLinkGateway(infra.FeedGateway)

	articleGW := global_search_gateway.NewArticleSearchGateway(infra.SearchIndexerDriver, urlGW)
	recapGW := global_search_gateway.NewRecapSearchGateway(infra.SearchIndexerDriver)
	tagGW := global_search_gateway.NewTagSearchGateway(infra.TagGateway)

	tracer := otel.Tracer(global_search_usecase.TracerName)

	return &SearchModule{
		GlobalSearchUsecase: global_search_usecase.NewGlobalSearchUsecase(articleGW, recapGW, tagGW, tracer),
	}
}

package di

import (
	"alt/orchestrator/gateway/feed_url_link_gateway"
	"alt/orchestrator/gateway/global_search_gateway"
	"alt/orchestrator/usecase/global_search_usecase"
	"alt/shared/driver/alt_db"

	"go.opentelemetry.io/otel"
)

// SearchModule holds all global-search-domain components.
type SearchModule struct {
	GlobalSearchUsecase *global_search_usecase.GlobalSearchUsecase
}

// newSearchModule creates the SearchModule and wires all global search components.
func newSearchModule(infra *InfraModule) *SearchModule {
	tagRepo := alt_db.NewTagRepository(infra.Pool)
	feedRepo := alt_db.NewFeedRepository(infra.Pool)
	urlGW := feed_url_link_gateway.NewFeedURLLinkGateway(feedRepo)

	articleGW := global_search_gateway.NewArticleSearchGateway(infra.SearchIndexerDriver, urlGW)
	recapGW := global_search_gateway.NewRecapSearchGateway(infra.SearchIndexerDriver)
	tagGW := global_search_gateway.NewTagSearchGateway(tagRepo)

	tracer := otel.Tracer(global_search_usecase.TracerName)

	return &SearchModule{
		GlobalSearchUsecase: global_search_usecase.NewGlobalSearchUsecase(articleGW, recapGW, tagGW, tracer),
	}
}

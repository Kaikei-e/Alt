package di

import (
	"alt/driver/csrf_token_driver"
	"alt/gateway/csrf_token_gateway"
	"alt/gateway/opml_gateway"
	"alt/gateway/subscription_gateway"
	"alt/usecase/csrf_token_usecase"
	"alt/usecase/feed_link_usecase"
	"alt/usecase/opml_usecase"
	"alt/usecase/subscription_usecase"
)

// SubscriptionModule holds subscription, OPML, and CSRF components.
type SubscriptionModule struct {
	// Subscription usecases
	ListSubscriptionsUsecase *subscription_usecase.ListSubscriptionsUsecase
	SubscribeUsecase         *subscription_usecase.SubscribeUsecase
	UnsubscribeUsecase       *subscription_usecase.UnsubscribeUsecase
	DeleteFeedLinkUsecase    *feed_link_usecase.DeleteFeedLinkUsecase

	// OPML usecases
	ExportOPMLUsecase *opml_usecase.ExportOPMLUsecase
	ImportOPMLUsecase *opml_usecase.ImportOPMLUsecase

	// CSRF usecase
	CSRFTokenUsecase *csrf_token_usecase.CSRFTokenUsecase

	// Gateway exposed for cross-module wiring
	SubscriptionGateway *subscription_gateway.SubscriptionGateway
}

func newSubscriptionModule(infra *InfraModule) *SubscriptionModule {
	pool := infra.Pool

	// Subscription
	subscriptionGw := subscription_gateway.NewSubscriptionGateway(pool)
	listSubscriptionsUC := subscription_usecase.NewListSubscriptionsUsecase(subscriptionGw)
	subscribeUC := subscription_usecase.NewSubscribeUsecase(subscriptionGw)
	unsubscribeUC := subscription_usecase.NewUnsubscribeUsecase(subscriptionGw)
	deleteFeedLinkUC := feed_link_usecase.NewDeleteFeedLinkUsecase(subscriptionGw)

	// OPML
	opmlExportGw := opml_gateway.NewExportGateway(pool)
	opmlImportGw := opml_gateway.NewImportGateway(pool)
	exportOPMLUC := opml_usecase.NewExportOPMLUsecase(opmlExportGw)
	importOPMLUC := opml_usecase.NewImportOPMLUsecase(opmlImportGw)

	// CSRF token
	csrfTokenDrv := csrf_token_driver.NewInMemoryCSRFTokenDriver()
	csrfTokenGw := csrf_token_gateway.NewCSRFTokenGateway(csrfTokenDrv)
	csrfTokenUC := csrf_token_usecase.NewCSRFTokenUsecase(csrfTokenGw)

	return &SubscriptionModule{
		ListSubscriptionsUsecase: listSubscriptionsUC,
		SubscribeUsecase:         subscribeUC,
		UnsubscribeUsecase:       unsubscribeUC,
		DeleteFeedLinkUsecase:    deleteFeedLinkUC,
		ExportOPMLUsecase:        exportOPMLUC,
		ImportOPMLUsecase:        importOPMLUC,
		CSRFTokenUsecase:         csrfTokenUC,
		SubscriptionGateway:      subscriptionGw,
	}
}

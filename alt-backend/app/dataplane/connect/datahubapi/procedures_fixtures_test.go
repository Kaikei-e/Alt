package datahubapi

import (
	"context"
	"testing"

	"alt/domain"
	"alt/mocks"
	"alt/shared/port/event_publisher_port"

	"go.uber.org/mock/gomock"
)

// testTenantID is the owner every CreateArticle test writes as. It is a real
// UUID rather than "system" or "user-123": the driver asserts that callers
// never reach into another tenant's rows, and a UUID is the only type it
// accepts.
const testTenantID = "00000000-0000-0000-0000-000000000001"

func setupHandler(t *testing.T) (
	*Handler,
	*mocks.MockListArticlesWithTagsPort,
	*mocks.MockListArticlesWithTagsForwardPort,
	*mocks.MockListDeletedArticlesPort,
	*mocks.MockGetLatestArticleTimestampPort,
	*mocks.MockGetArticleByIDPort,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	listArticles := mocks.NewMockListArticlesWithTagsPort(ctrl)
	listForward := mocks.NewMockListArticlesWithTagsForwardPort(ctrl)
	listDeleted := mocks.NewMockListDeletedArticlesPort(ctrl)
	getTimestamp := mocks.NewMockGetLatestArticleTimestampPort(ctrl)
	getByID := mocks.NewMockGetArticleByIDPort(ctrl)

	h := NewHandler(listArticles, listForward, listDeleted, getTimestamp, getByID, &fakeSystemUser{}, &fakeRecentArticles{}, nil)
	return h, listArticles, listForward, listDeleted, getTimestamp, getByID
}

// stubEventPublisher records published events and refuses to publish anything
// else. enabled is what the real gateway reads off its own client config, so
// the zero value is a wired publisher that is switched off — the state the
// handler must tell apart from a missing option.
type stubEventPublisher struct {
	enabled bool
	created []event_publisher_port.ArticleCreatedEvent
	updated []event_publisher_port.ArticleUpdatedEvent
}

func (s *stubEventPublisher) PublishArticleCreated(_ context.Context, e event_publisher_port.ArticleCreatedEvent) error {
	s.created = append(s.created, e)
	return nil
}

func (s *stubEventPublisher) PublishArticleUpdated(_ context.Context, e event_publisher_port.ArticleUpdatedEvent) error {
	s.updated = append(s.updated, e)
	return nil
}

func (s *stubEventPublisher) PublishSummarizeRequested(context.Context, event_publisher_port.SummarizeRequestedEvent) error {
	return nil
}

func (s *stubEventPublisher) PublishIndexArticle(context.Context, event_publisher_port.IndexArticleEvent) error {
	return nil
}

func (s *stubEventPublisher) IsEnabled() bool { return s.enabled }

type stubKnowledgeEventPort struct {
	called    bool
	lastEvent domain.KnowledgeEvent
	seq       int64
	err       error
}

func (s *stubKnowledgeEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	s.called = true
	s.lastEvent = event
	if s.err != nil {
		return 0, s.err
	}
	return s.seq, nil
}

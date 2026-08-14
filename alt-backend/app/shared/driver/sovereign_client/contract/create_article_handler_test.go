package contract

// The DataHubService handler the two CreateArticle pacts drive, and a guard
// that it can actually be driven.
//
// This file carries no `contract` build tag on purpose. The pact tests link
// libpact_ffi, so they only run where that library is installed; the handler
// they build is ordinary Go and its wiring can break without any of them
// running. CreateArticle panics on a dependency the composition root dropped
// (CLAUDE.md rule 8), and a pact whose handler panics on its first call fails
// as "no interaction received" on the one machine that can run it. The guard
// below builds the same handler through the same constructor and calls the
// same procedure, so the ordinary `go test ./...` sweep is where that is
// found.

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/dataplane/connect/datahubapi"
	"alt/dataplane/port/internal_article_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/orchestrator/usecase/fetch_recent_articles_usecase"
	"alt/shared/driver/mqhub_connect"
	"alt/shared/gateway/event_publisher_gateway"
	"alt/shared/port/knowledge_event_port"
)

// articleUpsertWriter answers CreateArticle the way the upsert does for an
// article alt-db already holds: same id, created=false.
type articleUpsertWriter struct {
	articleID string
	created   bool
}

func (w articleUpsertWriter) CreateArticle(context.Context, internal_article_port.CreateArticleParams) (string, bool, error) {
	return w.articleID, w.created, nil
}

// The two required constructor arguments of datahubapi.NewHandler. Neither
// procedure is exercised here; they exist because a handler cannot be built
// without them.
type unusedSystemUser struct{}

func (unusedSystemUser) GetFirstIdentityID(context.Context) (string, error) {
	return "", fmt.Errorf("GetSystemUser is not part of this contract")
}

type unusedRecentArticles struct{}

func (unusedRecentArticles) Execute(context.Context, fetch_recent_articles_usecase.FetchRecentArticlesInput) (*fetch_recent_articles_usecase.FetchRecentArticlesOutput, error) {
	return nil, fmt.Errorf("ListRecentArticles is not part of this contract")
}

// newCreateArticleHandler builds the data plane handler under contract, with
// the knowledge event sink the caller wants to observe and the article writer
// standing in for alt-db.
//
// The mq-hub publisher is wired even though no pact here covers a
// notification: it is a required dependency of CreateArticle, and "this
// contract is not about mq-hub" is said by handing over a publisher that
// reports itself off, never by omitting the option — the second one panics
// (CLAUDE.md rule 8 / ADR-000928). The production gateway over a disabled
// mqhub_connect.Client is that publisher: NewClient(_, false) builds no HTTP
// client and opens nothing, so it costs less than a stub and cannot drift
// from what the composition root wires when notifications are switched off.
func newCreateArticleHandler(
	appender knowledge_event_port.AppendKnowledgeEventPort,
	writer internal_article_port.CreateArticlePort,
) *datahubapi.Handler {
	return datahubapi.NewHandler(nil, nil, nil, nil, nil,
		unusedSystemUser{}, unusedRecentArticles{}, nil,
		datahubapi.WithPhase2Ports(nil, writer, nil, nil, nil, nil),
		datahubapi.WithKnowledgeEventPort(appender),
		datahubapi.WithEventPublisher(event_publisher_gateway.NewEventPublisherGateway(
			mqhub_connect.NewClient("", false), nil)),
	)
}

// recordingAppender accepts every event and keeps the last one.
type recordingAppender struct {
	last  domain.KnowledgeEvent
	calls int
}

func (a *recordingAppender) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	a.last = event
	a.calls++
	return 1, nil
}

// TestCreateArticleHandlerUnderContractIsFullyWired pins that the handler the
// CreateArticle pacts build answers the procedure they drive instead of
// panicking on a dependency nobody handed it.
func TestCreateArticleHandlerUnderContractIsFullyWired(t *testing.T) {
	const (
		tenantID  = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
		articleID = "ffffffff-ffff-ffff-ffff-ffffffffffff"
	)

	appender := &recordingAppender{}
	h := newCreateArticleHandler(appender, articleUpsertWriter{articleID: articleID, created: false})

	resp, err := h.CreateArticle(context.Background(), connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Knowledge Home must not show a blank title",
		Url:    "https://example.com/articles/blank-title",
		FeedId: "33333333-4444-5555-6666-777777777777",
		UserId: tenantID,
	}))
	require.NoError(t, err)
	assert.Equal(t, articleID, resp.Msg.GetArticleId())
	assert.Equal(t, 1, appender.calls, "the pact's single interaction is this append")
	assert.Equal(t, domain.EventArticleCreated, appender.last.EventType)
}

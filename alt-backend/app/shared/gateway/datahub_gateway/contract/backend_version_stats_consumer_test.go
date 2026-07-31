//go:build contract

// Pact CDC: alt-backend → alt-data-hub, versioned artifacts and dashboard
// statistics (ADR-000954 Wave 3 batch 5, capability catalog §2.K / §2.M).
//
// One consumer, like the read-state file: nothing here runs on a scheduler.
// Versions are appended when something is summarised or tagged, and the counts
// are read when somebody opens a dashboard.
//
// Two interactions in this file carry more weight than the rest.
//
// The first is MarkSummaryVersionSuperseded's absent-previous case. The
// procedure holds a per-article advisory lock across a select-then-update pair
// (catalog §2.K), and the only thing a consumer can observe of that
// transaction is what comes back: a previous version, or nothing. The caller
// emits SummarySuperseded on exactly that distinction, so a provider that
// returned a zero-valued message instead of an absent one would have the
// Knowledge Trail announce that a summary nobody wrote had been replaced —
// and every unit test on both sides would still pass, because both would be
// reading a struct that exists.
//
// The second is SaveArticleSummary's summaryVersioning field. That procedure
// appends a summary version as part of the write, which is right for
// pre-processor and wrong for the two alt-backend paths that append their own.
// The pact pins the field on the request so the provider cannot quietly go
// back to versioning unconditionally: the symptom of that regression is
// duplicate rows in a timeline, which nothing else here would catch.
package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"alt/domain"
	"alt/shared/gateway/datahub_gateway"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	versionArticleID   = "7a2b3c4d-5e6f-4a1b-8c2d-3e4f5a6b7c8d"
	summaryVersionID   = "11111111-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	prevSummaryID      = "22222222-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	tagSetVersionID    = "33333333-cccc-4ccc-8ccc-cccccccccccc"
	prevTagSetID       = "44444444-dddd-4ddd-8ddd-dddddddddddd"
	statsUserIDValue   = "9f8e7d6c-5b4a-4392-8281-706f5e4d3c2b"
	statsFeedIDValue   = "5c6d7e8f-9a0b-4c1d-8e2f-3a4b5c6d7e8f"
	versionGeneratedAt = "2026-07-31T09:00:00Z"
)

// statsContext puts a signed-in user on the context.
//
// Every §2.M gateway method except FeedAmount resolves the tenant here and
// sends it as a field, so a test that called them with a bare context would
// exercise the refusal path rather than the contract.
//
// Email and ExpiresAt are set because domain.UserContext.IsValid() requires
// them — a user id alone reads as an expired session, which is the same
// refusal an absent user gets.
func statsContext(t *testing.T) context.Context {
	t.Helper()
	userID, err := parseTestUUID(statsUserIDValue)
	require.NoError(t, err)
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "reader@example.com",
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

// ---------------------------------------------------------------------------
// §2.K Versioned artifacts — summary_versions
// ---------------------------------------------------------------------------

func TestCreateSummaryVersionContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	generatedAt, err := time.Parse(time.RFC3339, versionGeneratedAt)
	require.NoError(t, err)

	err = mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts summary version appends").
		UponReceiving("a CreateSummaryVersion request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/CreateSummaryVersion"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"version": map[string]interface{}{
					"summaryVersionId": summaryVersionID,
					"articleId":        versionArticleID,
					"userId":           statsUserIDValue,
					"generatedAt":      versionGeneratedAt,
					"model":            "stream-summarize",
					"summaryText":      "a summary",
				},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			// Append-only writes have nothing to report. The id was chosen by
			// the caller, so there is no server-assigned value to return.
			Body: matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewVersionGateway(newDataHubServiceClient(config))

			versionID, parseErr := parseTestUUID(summaryVersionID)
			if parseErr != nil {
				return parseErr
			}
			articleID, parseErr := parseTestUUID(versionArticleID)
			if parseErr != nil {
				return parseErr
			}
			userID, parseErr := parseTestUUID(statsUserIDValue)
			if parseErr != nil {
				return parseErr
			}

			// prompt_version, input_hash and quality_score are left unset on
			// purpose: they are absent for most real writes, and protojson
			// omits them. A pact recorded with them present would demand the
			// provider tolerate fields this caller does not send.
			if createErr := gw.CreateSummaryVersion(context.Background(), domain.SummaryVersion{
				SummaryVersionID: versionID,
				ArticleID:        articleID,
				UserID:           userID,
				GeneratedAt:      generatedAt,
				Model:            "stream-summarize",
				SummaryText:      "a summary",
				// Set, and deliberately not on the wire: ArticleTitle is
				// transport for the knowledge event, not a column.
				ArticleTitle: "The article",
			}); createErr != nil {
				return fmt.Errorf("CreateSummaryVersion failed: %w", createErr)
			}
			return nil
		})
	require.NoError(t, err)
}

// TestMarkSummaryVersionSupersededContract pins the answer the caller branches
// on when a previous version existed.
func TestMarkSummaryVersionSupersededContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has an earlier summary version for the article").
		UponReceiving("a MarkSummaryVersionSuperseded request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/MarkSummaryVersionSuperseded"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"articleId":    versionArticleID,
				"newVersionId": summaryVersionID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				// The version as it was *before* the update, which is what the
				// SummarySuperseded event excerpts. A provider that returned
				// the row after the UPDATE would carry supersededBy already
				// set and the excerpt would describe the new summary.
				"previousVersion": matchers.Like(map[string]interface{}{
					"summaryVersionId": matchers.Regex(prevSummaryID, uuidLikePattern),
					"articleId":        matchers.Regex(versionArticleID, uuidLikePattern),
					"userId":           matchers.Regex(statsUserIDValue, uuidLikePattern),
					"generatedAt":      matchers.Like("2026-07-30T09:00:00Z"),
					"model":            matchers.Like("pre-processor"),
					"summaryText":      matchers.Like("the older summary"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewVersionGateway(newDataHubServiceClient(config))

			articleID, parseErr := parseTestUUID(versionArticleID)
			if parseErr != nil {
				return parseErr
			}
			newID, parseErr := parseTestUUID(summaryVersionID)
			if parseErr != nil {
				return parseErr
			}

			prev, markErr := gw.MarkSummaryVersionSuperseded(context.Background(), articleID, newID)
			if markErr != nil {
				return fmt.Errorf("MarkSummaryVersionSuperseded failed: %w", markErr)
			}
			require.NotNil(t, prev)
			assert.Equal(t, prevSummaryID, prev.SummaryVersionID.String())
			assert.Equal(t, "the older summary", prev.SummaryText,
				"the excerpt in SummarySuperseded is cut from this text")
			return nil
		})
	require.NoError(t, err)
}

// TestMarkSummaryVersionSupersededFirstVersionContract is the interaction the
// whole append-first story rests on from the consumer's side.
//
// An article's first summary supersedes nothing, and the caller must be able to
// tell that from "there was a previous one". protojson encodes an unset message
// as an absent key, and the gateway maps the absence to a nil — anything else
// and the Trail gains a supersede event for a summary that never existed.
func TestMarkSummaryVersionSupersededFirstVersionContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no earlier summary version for the article").
		UponReceiving("a MarkSummaryVersionSuperseded request from alt-backend for a first summary").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/MarkSummaryVersionSuperseded"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"articleId":    "00000000-0000-4000-8000-000000000000",
				"newVersionId": summaryVersionID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewVersionGateway(newDataHubServiceClient(config))

			articleID, parseErr := parseTestUUID("00000000-0000-4000-8000-000000000000")
			if parseErr != nil {
				return parseErr
			}
			newID, parseErr := parseTestUUID(summaryVersionID)
			if parseErr != nil {
				return parseErr
			}

			prev, markErr := gw.MarkSummaryVersionSuperseded(context.Background(), articleID, newID)
			if markErr != nil {
				return fmt.Errorf("MarkSummaryVersionSuperseded failed: %w", markErr)
			}
			assert.Nil(t, prev, "a first version supersedes nothing, and that must not arrive as a zero value")
			return nil
		})
	require.NoError(t, err)
}

// TestGetSummaryVersionByIDContract pins the reproject-safe read: asked for a
// specific id, the provider returns that version even though it has been
// superseded since.
func TestGetSummaryVersionByIDContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a superseded summary version").
		UponReceiving("a GetSummaryVersionByID request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetSummaryVersionByID"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"summaryVersionId": prevSummaryID},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"version": matchers.Like(map[string]interface{}{
					"summaryVersionId": matchers.Regex(prevSummaryID, uuidLikePattern),
					"articleId":        matchers.Regex(versionArticleID, uuidLikePattern),
					"userId":           matchers.Regex(statsUserIDValue, uuidLikePattern),
					"generatedAt":      matchers.Like("2026-07-30T09:00:00Z"),
					"model":            matchers.Like("pre-processor"),
					"summaryText":      matchers.Like("the older summary"),
					// Present, because this version has been replaced. The read
					// still returns it: that is what "reproject-safe" means
					// here — an old event resolves to what it was about.
					"supersededBy": matchers.Regex(summaryVersionID, uuidLikePattern),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewVersionGateway(newDataHubServiceClient(config))

			versionID, parseErr := parseTestUUID(prevSummaryID)
			if parseErr != nil {
				return parseErr
			}

			sv, getErr := gw.GetSummaryVersionByID(context.Background(), versionID)
			if getErr != nil {
				return fmt.Errorf("GetSummaryVersionByID failed: %w", getErr)
			}
			assert.Equal(t, prevSummaryID, sv.SummaryVersionID.String())
			require.NotNil(t, sv.SupersededBy)
			assert.Equal(t, summaryVersionID, sv.SupersededBy.String())
			return nil
		})
	require.NoError(t, err)
}

func TestGetLatestSummaryVersionContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a current summary version for the article").
		UponReceiving("a GetLatestSummaryVersion request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetLatestSummaryVersion"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"articleId": versionArticleID},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				// No supersededBy: "latest" means exactly "nothing has replaced
				// it", so a current version that carried one would be a
				// contradiction rather than extra detail.
				"version": matchers.Like(map[string]interface{}{
					"summaryVersionId": matchers.Regex(summaryVersionID, uuidLikePattern),
					"articleId":        matchers.Regex(versionArticleID, uuidLikePattern),
					"userId":           matchers.Regex(statsUserIDValue, uuidLikePattern),
					"generatedAt":      matchers.Like(versionGeneratedAt),
					"model":            matchers.Like("stream-summarize"),
					"summaryText":      matchers.Like("a summary"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewVersionGateway(newDataHubServiceClient(config))

			articleID, parseErr := parseTestUUID(versionArticleID)
			if parseErr != nil {
				return parseErr
			}

			sv, getErr := gw.GetLatestSummaryVersion(context.Background(), articleID)
			if getErr != nil {
				return fmt.Errorf("GetLatestSummaryVersion failed: %w", getErr)
			}
			assert.Equal(t, summaryVersionID, sv.SummaryVersionID.String())
			assert.Nil(t, sv.SupersededBy)
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.K Versioned artifacts — tag_set_versions
// ---------------------------------------------------------------------------

// TestCreateTagSetVersionContract pins the tags_json encoding.
//
// protoJSON renders a bytes field as base64, and the value below decodes to
// [{"name":"AI"}]. That is part of the contract rather than an implementation
// detail: the column is jsonb the generator wrote, and re-encoding it through a
// schema on either side would make a later read return something the generator
// did not produce.
func TestCreateTagSetVersionContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	generatedAt, err := time.Parse(time.RFC3339, versionGeneratedAt)
	require.NoError(t, err)

	err = mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts tag set version appends").
		UponReceiving("a CreateTagSetVersion request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/CreateTagSetVersion"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"version": map[string]interface{}{
					"tagSetVersionId": tagSetVersionID,
					"articleId":       versionArticleID,
					"userId":          statsUserIDValue,
					"generatedAt":     versionGeneratedAt,
					"generator":       "tag-generator",
					"tagsJson":        "W3sibmFtZSI6IkFJIn1d",
				},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewVersionGateway(newDataHubServiceClient(config))

			versionID, parseErr := parseTestUUID(tagSetVersionID)
			if parseErr != nil {
				return parseErr
			}
			articleID, parseErr := parseTestUUID(versionArticleID)
			if parseErr != nil {
				return parseErr
			}
			userID, parseErr := parseTestUUID(statsUserIDValue)
			if parseErr != nil {
				return parseErr
			}

			if createErr := gw.CreateTagSetVersion(context.Background(), domain.TagSetVersion{
				TagSetVersionID: versionID,
				ArticleID:       articleID,
				UserID:          userID,
				GeneratedAt:     generatedAt,
				Generator:       "tag-generator",
				TagsJSON:        json.RawMessage(`[{"name":"AI"}]`),
			}); createErr != nil {
				return fmt.Errorf("CreateTagSetVersion failed: %w", createErr)
			}
			return nil
		})
	require.NoError(t, err)
}

func TestMarkTagSetVersionSupersededContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has an earlier tag set version for the article").
		UponReceiving("a MarkTagSetVersionSuperseded request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/MarkTagSetVersionSuperseded"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"articleId":    versionArticleID,
				"newVersionId": tagSetVersionID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"previousVersion": matchers.Like(map[string]interface{}{
					"tagSetVersionId": matchers.Regex(prevTagSetID, uuidLikePattern),
					"articleId":       matchers.Regex(versionArticleID, uuidLikePattern),
					"userId":          matchers.Regex(statsUserIDValue, uuidLikePattern),
					"generatedAt":     matchers.Like("2026-07-30T09:00:00Z"),
					"generator":       matchers.Like("tag-generator"),
					"tagsJson":        matchers.Like("W3sibmFtZSI6IkFJIn1d"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewVersionGateway(newDataHubServiceClient(config))

			articleID, parseErr := parseTestUUID(versionArticleID)
			if parseErr != nil {
				return parseErr
			}
			newID, parseErr := parseTestUUID(tagSetVersionID)
			if parseErr != nil {
				return parseErr
			}

			prev, markErr := gw.MarkTagSetVersionSuperseded(context.Background(), articleID, newID)
			if markErr != nil {
				return fmt.Errorf("MarkTagSetVersionSuperseded failed: %w", markErr)
			}
			require.NotNil(t, prev)
			assert.Equal(t, prevTagSetID, prev.TagSetVersionID.String())
			assert.JSONEq(t, `[{"name":"AI"}]`, string(prev.TagsJSON))
			return nil
		})
	require.NoError(t, err)
}

func TestGetTagSetVersionByIDContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a tag set version").
		UponReceiving("a GetTagSetVersionByID request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetTagSetVersionByID"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"tagSetVersionId": tagSetVersionID},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"version": matchers.Like(map[string]interface{}{
					"tagSetVersionId": matchers.Regex(tagSetVersionID, uuidLikePattern),
					"articleId":       matchers.Regex(versionArticleID, uuidLikePattern),
					"userId":          matchers.Regex(statsUserIDValue, uuidLikePattern),
					"generatedAt":     matchers.Like(versionGeneratedAt),
					"generator":       matchers.Like("tag-generator"),
					"tagsJson":        matchers.Like("W3sibmFtZSI6IkFJIn1d"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewVersionGateway(newDataHubServiceClient(config))

			versionID, parseErr := parseTestUUID(tagSetVersionID)
			if parseErr != nil {
				return parseErr
			}

			tsv, getErr := gw.GetTagSetVersionByID(context.Background(), versionID)
			if getErr != nil {
				return fmt.Errorf("GetTagSetVersionByID failed: %w", getErr)
			}
			assert.Equal(t, tagSetVersionID, tsv.TagSetVersionID.String())
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.K seam — the article_summaries write
// ---------------------------------------------------------------------------

// TestSaveArticleSummaryFromBackendContract pins the two fields that let
// alt-backend stop writing article_summaries directly.
//
// articleTitle is present because this caller has one and the driver stored it.
// summaryVersioning is SKIP because this caller appends its own version through
// CreateSummaryVersion; without it the provider would append a second one, and
// the only visible symptom would be a duplicated entry in a Knowledge Home
// timeline nobody diffs.
func TestSaveArticleSummaryFromBackendContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts article summary writes").
		UponReceiving("a SaveArticleSummary request from alt-backend that owns its own versioning").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/SaveArticleSummary"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"articleId":         versionArticleID,
				"userId":            statsUserIDValue,
				"articleTitle":      "The article",
				"summary":           "a summary",
				"summaryVersioning": "SUMMARY_VERSIONING_SKIP",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"success": matchers.Like(true),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewArticleStoreGateway(newDataHubServiceClient(config))
			if saveErr := gw.SaveArticleSummary(context.Background(),
				versionArticleID, statsUserIDValue, "The article", "a summary"); saveErr != nil {
				return fmt.Errorf("SaveArticleSummary failed: %w", saveErr)
			}
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.M Statistics / dashboard
// ---------------------------------------------------------------------------

// TestGetFeedAmountContract is the one count with no tenant. Its request body
// is empty, and that emptiness is the contract: a userId appearing here later
// would mean the number had quietly become per-user.
func TestGetFeedAmountContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has feeds").
		UponReceiving("a GetFeedAmount request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetFeedAmount"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"count": matchers.Like(42)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			count, countErr := gw.FeedAmount(context.Background())
			if countErr != nil {
				return fmt.Errorf("FeedAmount failed: %w", countErr)
			}
			assert.Equal(t, 42, count)
			return nil
		})
	require.NoError(t, err)
}

// TestGetTotalArticlesCountContract pins the tenant field on the tenant-scoped
// counts.
//
// The driver read the user from the request context; over Connect the peer
// certificate names alt-backend and nothing about whose articles these are, so
// the field is the whole tenancy story. A provider that ignored it would answer
// every user with the same number and no test that mocked the gateway would
// notice.
func TestGetTotalArticlesCountContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has articles for the user").
		UponReceiving("a GetTotalArticlesCount request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetTotalArticlesCount"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"userId": statsUserIDValue},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"count": matchers.Like(120)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			count, countErr := gw.TotalArticlesCount(statsContext(t))
			if countErr != nil {
				return fmt.Errorf("TotalArticlesCount failed: %w", countErr)
			}
			assert.Equal(t, 120, count)
			return nil
		})
	require.NoError(t, err)
}

func TestGetSummarizedArticlesCountContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has summarized articles for the user").
		UponReceiving("a GetSummarizedArticlesCount request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetSummarizedArticlesCount"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"userId": statsUserIDValue},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"count": matchers.Like(80)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			count, countErr := gw.SummarizedArticlesCount(statsContext(t))
			if countErr != nil {
				return fmt.Errorf("SummarizedArticlesCount failed: %w", countErr)
			}
			assert.Equal(t, 80, count)
			return nil
		})
	require.NoError(t, err)
}

// TestGetUnsummarizedArticlesCountContract exists as its own interaction rather
// than being derived from the two counts above, because it is its own query: a
// summary can outlive the article it describes, so total minus summarized is
// not this number.
func TestGetUnsummarizedArticlesCountContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has unsummarized articles for the user").
		UponReceiving("a GetUnsummarizedArticlesCount request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetUnsummarizedArticlesCount"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"userId": statsUserIDValue},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"count": matchers.Like(40)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			count, countErr := gw.UnsummarizedArticlesCount(statsContext(t))
			if countErr != nil {
				return fmt.Errorf("UnsummarizedArticlesCount failed: %w", countErr)
			}
			assert.Equal(t, 40, count)
			return nil
		})
	require.NoError(t, err)
}

// TestGetTodayUnreadArticlesCountContract pins `since` on the request. "Today"
// is a wall-clock question and the provider has no timezone for it, so the
// bound travels rather than being derived on the far side.
func TestGetTodayUnreadArticlesCountContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	since := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has unread feeds for the user since the bound").
		UponReceiving("a GetTodayUnreadArticlesCount request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetTodayUnreadArticlesCount"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId": statsUserIDValue,
				"since":  "2026-07-31T00:00:00Z",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"count": matchers.Like(7)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			count, countErr := gw.TodayUnreadArticlesCount(statsContext(t), since)
			if countErr != nil {
				return fmt.Errorf("TodayUnreadArticlesCount failed: %w", countErr)
			}
			assert.Equal(t, 7, count)
			return nil
		})
	require.NoError(t, err)
}

// TestGetTrendStatsContract pins the window enum and the granularity that comes
// back with it.
//
// The two are not independent: a 7-day window is bucketed daily because that is
// what the query's date_trunc does, and the caller labels its axis from the
// answer rather than from its own request. Sending the window as an enum is
// what keeps "90d" from being a runtime error on a value the contract had
// implied was acceptable.
func TestGetTrendStatsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has trend data for the user").
		UponReceiving("a GetTrendStats request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetTrendStats"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId": statsUserIDValue,
				"window": "TREND_WINDOW_7D",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"points": matchers.EachLike(map[string]interface{}{
					"bucket":       matchers.Like("2026-07-30T00:00:00Z"),
					"articles":     matchers.Like(12),
					"summarized":   matchers.Like(9),
					"feedActivity": matchers.Like(3),
				}, 1),
				"granularity": matchers.Like("TREND_GRANULARITY_DAILY"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			series, statsErr := gw.TrendStats(statsContext(t), "7d")
			if statsErr != nil {
				return fmt.Errorf("TrendStats failed: %w", statsErr)
			}
			require.Len(t, series.Points, 1)
			assert.Equal(t, 12, series.Points[0].Articles)
			assert.Equal(t, "daily", series.Granularity)
			return nil
		})
	require.NoError(t, err)
}

func TestListUserFeedIDsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has read state for the user").
		UponReceiving("a ListUserFeedIDs request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/ListUserFeedIDs"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"userId": statsUserIDValue},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"feedIds": matchers.EachLike(matchers.Regex(statsFeedIDValue, uuidLikePattern), 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewStatsGateway(newDataHubServiceClient(config))
			ids, listErr := gw.UserFeedIDs(statsContext(t))
			if listErr != nil {
				return fmt.Errorf("UserFeedIDs failed: %w", listErr)
			}
			require.Len(t, ids, 1)
			assert.Equal(t, statsFeedIDValue, ids[0].String())
			return nil
		})
	require.NoError(t, err)
}

package create_summary_version_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSummaryPort struct {
	created []domain.SummaryVersion
	err     error
}

func (m *mockSummaryPort) CreateSummaryVersion(_ context.Context, sv domain.SummaryVersion) error {
	if m.err != nil {
		return m.err
	}
	m.created = append(m.created, sv)
	return nil
}

type mockEventPort struct {
	appended []domain.KnowledgeEvent
	err      error
}

func (m *mockEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	m.appended = append(m.appended, event)
	return int64(len(m.appended)), nil
}

type mockMarkSupersededPort struct {
	prev *domain.SummaryVersion
	err  error
}

func (m *mockMarkSupersededPort) MarkSummaryVersionSuperseded(_ context.Context, _ uuid.UUID, _ uuid.UUID) (*domain.SummaryVersion, error) {
	return m.prev, m.err
}

func TestCreateSummaryVersionUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	articleID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name        string
		sv          domain.SummaryVersion
		summaryPort *mockSummaryPort
		eventPort   *mockEventPort
		wantErr     bool
	}{
		{
			name: "success - creates version and emits event",
			sv: domain.SummaryVersion{
				ArticleID:   articleID,
				UserID:      userID,
				Model:       "gpt-4",
				SummaryText: "Test summary",
			},
			summaryPort: &mockSummaryPort{},
			eventPort:   &mockEventPort{},
		},
		{
			name: "error - missing article_id",
			sv: domain.SummaryVersion{
				SummaryText: "Test summary",
			},
			summaryPort: &mockSummaryPort{},
			eventPort:   &mockEventPort{},
			wantErr:     true,
		},
		{
			name: "error - missing summary_text",
			sv: domain.SummaryVersion{
				ArticleID: articleID,
			},
			summaryPort: &mockSummaryPort{},
			eventPort:   &mockEventPort{},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewCreateSummaryVersionUsecase(tt.summaryPort, tt.eventPort, nil)
			err := uc.Execute(context.Background(), tt.sv)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, tt.summaryPort.created, 1)
			assert.Len(t, tt.eventPort.appended, 1)
			assert.Equal(t, domain.EventSummaryVersionCreated, tt.eventPort.appended[0].EventType)
		})
	}
}

func TestCreateSummaryVersionUsecase_PayloadCarriesArticleTitle(t *testing.T) {
	logger.InitLogger()

	summaryPort := &mockSummaryPort{}
	eventPort := &mockEventPort{}
	uc := NewCreateSummaryVersionUsecase(summaryPort, eventPort, nil)

	err := uc.Execute(context.Background(), domain.SummaryVersion{
		ArticleID:    uuid.New(),
		UserID:       uuid.New(),
		SummaryText:  "Test summary",
		ArticleTitle: "How Event Sourcing Changes Everything",
	})
	require.NoError(t, err)
	require.Len(t, eventPort.appended, 1)

	// The Knowledge Loop projector's reproject-safe enricher reads
	// payload.article_title to render a substantive narrative; the producer
	// (this usecase) is the only place the title can be captured at the
	// event-time invariant. Missing title would force the enricher to fall
	// back to the generic feed-level sentence — the production regression
	// the user reported as "all 3 cards say A new summary is ready in one
	// of your feeds.".
	var payload map[string]string
	require.NoError(t, json.Unmarshal(eventPort.appended[0].Payload, &payload))
	require.Equal(t, "How Event Sourcing Changes Everything", payload["article_title"])
}

func TestCreateSummaryVersionUsecase_PayloadOmitsEmptyArticleTitle(t *testing.T) {
	logger.InitLogger()

	summaryPort := &mockSummaryPort{}
	eventPort := &mockEventPort{}
	uc := NewCreateSummaryVersionUsecase(summaryPort, eventPort, nil)

	err := uc.Execute(context.Background(), domain.SummaryVersion{
		ArticleID:   uuid.New(),
		UserID:      uuid.New(),
		SummaryText: "Test summary",
	})
	require.NoError(t, err)
	require.Len(t, eventPort.appended, 1)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(eventPort.appended[0].Payload, &payload))
	_, present := payload["article_title"]
	require.False(t, present, "empty title is omitted so the enricher's default-branch fallback engages")
}

func TestCreateSummaryVersionUsecase_FirstVersion_NoSupersedeEvent(t *testing.T) {
	logger.InitLogger()

	summaryPort := &mockSummaryPort{}
	eventPort := &mockEventPort{}
	markPort := &mockMarkSupersededPort{prev: nil} // No previous version

	uc := NewCreateSummaryVersionUsecase(summaryPort, eventPort, markPort)
	err := uc.Execute(context.Background(), domain.SummaryVersion{
		ArticleID:   uuid.New(),
		UserID:      uuid.New(),
		SummaryText: "First summary",
	})

	require.NoError(t, err)
	assert.Len(t, eventPort.appended, 1, "first version: only SummaryVersionCreated, no SummarySuperseded")
	assert.Equal(t, domain.EventSummaryVersionCreated, eventPort.appended[0].EventType)
}

func TestCreateSummaryVersionUsecase_SecondVersion_EmitsSupersedeEvent(t *testing.T) {
	logger.InitLogger()

	articleID := uuid.New()
	userID := uuid.New()
	oldVersionID := uuid.New()

	summaryPort := &mockSummaryPort{}
	eventPort := &mockEventPort{}
	markPort := &mockMarkSupersededPort{
		prev: &domain.SummaryVersion{
			SummaryVersionID: oldVersionID,
			ArticleID:        articleID,
			UserID:           userID,
			SummaryText:      "Old summary text that was previously the latest version",
		},
	}

	uc := NewCreateSummaryVersionUsecase(summaryPort, eventPort, markPort)
	err := uc.Execute(context.Background(), domain.SummaryVersion{
		ArticleID:   articleID,
		UserID:      userID,
		SummaryText: "New improved summary",
	})

	require.NoError(t, err)
	require.Len(t, eventPort.appended, 2, "second version: SummaryVersionCreated + SummarySuperseded")
	assert.Equal(t, domain.EventSummaryVersionCreated, eventPort.appended[0].EventType)
	assert.Equal(t, domain.EventSummarySuperseded, eventPort.appended[1].EventType)

	// Verify supersede event payload contains previous excerpt
	assert.Contains(t, string(eventPort.appended[1].Payload), "previous_summary_excerpt")
	assert.Contains(t, string(eventPort.appended[1].Payload), "Old summary text")
}

func TestCreateSummaryVersionUsecase_SecondVersion_TruncatesPreviousExcerptUTF8Safely(t *testing.T) {
	logger.InitLogger()

	articleID := uuid.New()
	userID := uuid.New()
	oldVersionID := uuid.New()

	longJapaneseSummary := "あ"
	for len([]rune(longJapaneseSummary)) <= maxPreviousExcerptLen {
		longJapaneseSummary += "更新"
	}

	summaryPort := &mockSummaryPort{}
	eventPort := &mockEventPort{}
	markPort := &mockMarkSupersededPort{
		prev: &domain.SummaryVersion{
			SummaryVersionID: oldVersionID,
			ArticleID:        articleID,
			UserID:           userID,
			SummaryText:      longJapaneseSummary,
		},
	}

	uc := NewCreateSummaryVersionUsecase(summaryPort, eventPort, markPort)
	err := uc.Execute(context.Background(), domain.SummaryVersion{
		ArticleID:   articleID,
		UserID:      userID,
		SummaryText: "New improved summary",
	})

	require.NoError(t, err)
	require.Len(t, eventPort.appended, 2)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(eventPort.appended[1].Payload, &payload))

	previousExcerpt := payload["previous_summary_excerpt"]
	assert.NotEmpty(t, previousExcerpt)
	assert.True(t, utf8.ValidString(previousExcerpt), "previous excerpt must remain valid UTF-8")
	assert.LessOrEqual(t, len([]rune(previousExcerpt)), maxPreviousExcerptLen+1)
}

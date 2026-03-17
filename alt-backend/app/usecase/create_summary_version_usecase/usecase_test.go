package create_summary_version_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"testing"

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

func (m *mockEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) error {
	if m.err != nil {
		return m.err
	}
	m.appended = append(m.appended, event)
	return nil
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
			uc := NewCreateSummaryVersionUsecase(tt.summaryPort, tt.eventPort)
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

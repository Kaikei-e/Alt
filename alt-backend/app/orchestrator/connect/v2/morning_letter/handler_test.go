package morning_letter

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"

	"alt/domain"
	morningletterv2 "alt/gen/proto/alt/morning_letter/v2"
	"alt/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func testCtxWithUser() context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

func TestGetLatestLetter_ReturnsLetter(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUC := mocks.NewMockMorningLetterUsecase(ctrl)
	mockChat := mocks.NewMockStreamChatPort(ctrl)

	doc := &domain.MorningLetterDocument{
		ID:              "test-id",
		TargetDate:      "2026-04-07",
		EditionTimezone: "Asia/Tokyo",
		IsDegraded:      false,
		SchemaVersion:   1,
		Model:           "gemma4-e4b-12k",
		CreatedAt:       time.Date(2026, 4, 7, 6, 0, 0, 0, time.UTC),
		Etag:            "\"test:1\"",
		Body: domain.MorningLetterBody{
			Lead:        "Today's briefing",
			GeneratedAt: time.Date(2026, 4, 7, 5, 30, 0, 0, time.UTC),
			Sections: []domain.MorningLetterSection{
				{Key: "top3", Title: "Top Stories", Bullets: []string{"Bullet 1"}, Genre: ""},
			},
		},
	}
	mockUC.EXPECT().GetLatestLetter(gomock.Any()).Return(doc, nil)

	h := NewHandler(mockChat, mockUC, slog.Default())
	resp, err := h.GetLatestLetter(testCtxWithUser(), connect.NewRequest(&morningletterv2.GetLatestLetterRequest{}))

	require.NoError(t, err)
	letter := resp.Msg.Letter
	assert.Equal(t, "test-id", letter.Id)
	assert.Equal(t, "Today's briefing", letter.Body.Lead)
	assert.Len(t, letter.Body.Sections, 1)
	assert.Equal(t, "\"test:1\"", letter.Etag)
}

func TestGetLatestLetter_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUC := mocks.NewMockMorningLetterUsecase(ctrl)
	mockChat := mocks.NewMockStreamChatPort(ctrl)

	mockUC.EXPECT().GetLatestLetter(gomock.Any()).Return(nil, nil)

	h := NewHandler(mockChat, mockUC, slog.Default())
	_, err := h.GetLatestLetter(testCtxWithUser(), connect.NewRequest(&morningletterv2.GetLatestLetterRequest{}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestGetLatestLetter_RequiresAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUC := mocks.NewMockMorningLetterUsecase(ctrl)
	mockChat := mocks.NewMockStreamChatPort(ctrl)

	h := NewHandler(mockChat, mockUC, slog.Default())
	_, err := h.GetLatestLetter(context.Background(), connect.NewRequest(&morningletterv2.GetLatestLetterRequest{}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestGetLetterByDate_ReturnsLetter(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUC := mocks.NewMockMorningLetterUsecase(ctrl)
	mockChat := mocks.NewMockStreamChatPort(ctrl)

	doc := &domain.MorningLetterDocument{
		ID:         "date-id",
		TargetDate: "2026-04-07",
		Body: domain.MorningLetterBody{
			Lead: "Date letter",
		},
	}
	mockUC.EXPECT().GetLetterByDate(gomock.Any(), "2026-04-07").Return(doc, nil)

	h := NewHandler(mockChat, mockUC, slog.Default())
	resp, err := h.GetLetterByDate(testCtxWithUser(), connect.NewRequest(&morningletterv2.GetLetterByDateRequest{
		TargetDate: "2026-04-07",
	}))

	require.NoError(t, err)
	assert.Equal(t, "date-id", resp.Msg.Letter.Id)
}

func TestGetLetterByDate_EmptyDate(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUC := mocks.NewMockMorningLetterUsecase(ctrl)
	mockChat := mocks.NewMockStreamChatPort(ctrl)

	h := NewHandler(mockChat, mockUC, slog.Default())
	_, err := h.GetLetterByDate(testCtxWithUser(), connect.NewRequest(&morningletterv2.GetLetterByDateRequest{
		TargetDate: "",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestGetLetterSources_ReturnsFilteredSources(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUC := mocks.NewMockMorningLetterUsecase(ctrl)
	mockChat := mocks.NewMockStreamChatPort(ctrl)

	sources := []*domain.MorningLetterSourceEntry{
		{LetterID: "l1", SectionKey: "top3", SourceType: "recap", Position: 0},
		{LetterID: "l1", SectionKey: "top3", SourceType: "overnight", Position: 1},
		{LetterID: "l1", SectionKey: "what_changed", SourceType: "recap", Position: 0},
	}
	mockUC.EXPECT().GetLetterSources(gomock.Any(), "l1").Return(sources, nil)

	h := NewHandler(mockChat, mockUC, slog.Default())
	resp, err := h.GetLetterSources(testCtxWithUser(), connect.NewRequest(&morningletterv2.GetLetterSourcesRequest{
		LetterId: "l1",
	}))

	require.NoError(t, err)
	assert.Len(t, resp.Msg.Sources, 3)
}

func TestGetLetterSources_EmptyLetterID(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUC := mocks.NewMockMorningLetterUsecase(ctrl)
	mockChat := mocks.NewMockStreamChatPort(ctrl)

	h := NewHandler(mockChat, mockUC, slog.Default())
	_, err := h.GetLetterSources(testCtxWithUser(), connect.NewRequest(&morningletterv2.GetLetterSourcesRequest{
		LetterId: "",
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

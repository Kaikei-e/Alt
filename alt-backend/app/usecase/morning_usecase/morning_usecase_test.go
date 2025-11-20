package morning_usecase_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"alt/domain"
	"alt/mocks"
	"alt/usecase/morning_usecase"
)

func TestGetOvernightUpdates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockMorningRepository(ctrl)
	mockUserFeedPort := mocks.NewMockUserFeedPort(ctrl)
	usecase := morning_usecase.NewMorningUsecase(mockRepo, mockUserFeedPort)

	ctx := context.Background()
	userIDStr := "11111111-1111-1111-1111-111111111111"
	userID := uuid.MustParse(userIDStr)

	// Mock data
	groupID := uuid.New()
	articleID1 := uuid.New()
	articleID2 := uuid.New()
	feedID := uuid.New()

	groups := []*domain.MorningArticleGroup{
		{
			GroupID:   groupID,
			ArticleID: articleID1,
			IsPrimary: true,
			Article: &domain.Article{
				ID:     articleID1,
				FeedID: feedID,
				Title:  "Primary Article",
			},
		},
		{
			GroupID:   groupID,
			ArticleID: articleID2,
			IsPrimary: false,
			Article: &domain.Article{
				ID:     articleID2,
				FeedID: feedID,
				Title:  "Duplicate Article",
			},
		},
	}

	// Expectation for user feed IDs
	mockUserFeedPort.EXPECT().
		GetUserFeedIDs(ctx, userID).
		Return([]uuid.UUID{feedID}, nil)

	// Expectation for morning article groups
	mockRepo.EXPECT().
		GetMorningArticleGroups(ctx, gomock.Any()). // time only
		Return(groups, nil)

	// Execute
	updates, err := usecase.GetOvernightUpdates(ctx, userIDStr)

	// Verify
	assert.NoError(t, err)
	assert.Len(t, updates, 1)
	assert.Equal(t, groupID, updates[0].GroupID)
	assert.Equal(t, "Primary Article", updates[0].PrimaryArticle.Title)
	assert.Len(t, updates[0].Duplicates, 1)
	assert.Equal(t, "Duplicate Article", updates[0].Duplicates[0].Title)
}

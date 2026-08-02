package datahub_gateway

import (
	"context"
	"testing"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestUserIDFromContext(t *testing.T) {
	userID := uuid.New()

	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{
			name: "renders the signed-in user",
			ctx: domain.SetUserContext(context.Background(), &domain.UserContext{
				UserID:    userID,
				Email:     "a@example.com",
				Role:      domain.UserRoleUser,
				TenantID:  uuid.New(),
				ExpiresAt: time.Now().Add(time.Hour),
			}),
			want: userID.String(),
		},
		{
			name: "no user is the empty string, which the provider reads as unscoped",
			ctx:  context.Background(),
			want: "",
		},
		{
			name: "an expired context is not a user either",
			ctx: domain.SetUserContext(context.Background(), &domain.UserContext{
				UserID:    userID,
				Email:     "a@example.com",
				ExpiresAt: time.Now().Add(-time.Hour),
			}),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, userIDFromContext(tt.ctx))
		})
	}
}

// A nil message means "no such article". Mapping it to an empty struct would
// turn "not archived yet" into "archived with no body", and the fetch path
// branches on exactly that difference.
func TestArticleContentFromProto_NilStaysNil(t *testing.T) {
	assert.Nil(t, articleContentFromProto(nil))
}

func TestArticleContentFromProto(t *testing.T) {
	got := articleContentFromProto(&datahubv1.ArticleContent{
		Id:      "a1",
		Title:   "T",
		Content: "C",
		Url:     "https://example.com/a",
		FeedId:  "f1",
	})
	require.NotNil(t, got)
	assert.Equal(t, &domain.ArticleContent{ID: "a1", Title: "T", Content: "C", URL: "https://example.com/a", FeedID: "f1"}, got)
}

func TestUserArticleFromProto(t *testing.T) {
	id := uuid.New()
	feedID := uuid.New()
	ts := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

	got, err := userArticleFromProto(&datahubv1.UserArticle{
		Id:          id.String(),
		FeedId:      feedID.String(),
		Title:       "T",
		Content:     "C",
		Url:         "https://example.com/a",
		Tags:        []string{"go", "rust"},
		PublishedAt: timestamppb.New(ts),
		CreatedAt:   timestamppb.New(ts),
	})
	require.NoError(t, err)
	assert.Equal(t, id, got.ID)
	assert.Equal(t, feedID, got.FeedID)
	assert.Equal(t, []string{"go", "rust"}, got.Tags)
	assert.True(t, ts.Equal(got.CreatedAt))
	assert.True(t, ts.Equal(got.UpdatedAt), "articles has no updated_at column; the driver used created_at and so does this")
}

// An article whose feed was never resolved is a real state, not a malformed
// message: the column is nullable and the driver read it as the nil UUID.
func TestUserArticleFromProto_EmptyFeedIDIsNilUUID(t *testing.T) {
	got, err := userArticleFromProto(&datahubv1.UserArticle{Id: uuid.NewString()})
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, got.FeedID)
}

// An unparseable article id is an error rather than the nil UUID. The read
// model behind this is a cache keyed by article id: every malformed row would
// collide on the same key and serve the previous one's body.
func TestUserArticleFromProto_RejectsUnparseableID(t *testing.T) {
	_, err := userArticleFromProto(&datahubv1.UserArticle{Id: "not-a-uuid"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a uuid")
}

func TestBackfillArticleFromProto(t *testing.T) {
	articleID, userID := uuid.New(), uuid.New()
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	got, err := backfillArticleFromProto(&datahubv1.BackfillArticle{
		ArticleId:   articleID.String(),
		UserId:      userID.String(),
		CreatedAt:   timestamppb.New(createdAt),
		PublishedAt: timestamppb.New(createdAt),
		Title:       "T",
		Url:         "https://example.com/a",
	})
	require.NoError(t, err)
	assert.Equal(t, articleID, got.ArticleID)
	assert.Equal(t, userID, got.UserID)
	assert.True(t, createdAt.Equal(got.PublishedAt))
}

func TestBackfillArticleFromProto_RejectsUnparseableIDs(t *testing.T) {
	tests := []struct {
		name string
		msg  *datahubv1.BackfillArticle
	}{
		{"article id", &datahubv1.BackfillArticle{ArticleId: "x", UserId: uuid.NewString()}},
		{"user id", &datahubv1.BackfillArticle{ArticleId: uuid.NewString(), UserId: "x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := backfillArticleFromProto(tt.msg)
			require.Error(t, err, "a zero id here would append a knowledge event attributed to nobody")
		})
	}
}

func TestBackfillSummaryTitleFromProto(t *testing.T) {
	versionID, articleID, userID := uuid.New(), uuid.New(), uuid.New()
	generatedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	got, err := backfillSummaryTitleFromProto(&datahubv1.BackfillSummaryTitle{
		SummaryVersionId: versionID.String(),
		ArticleId:        articleID.String(),
		UserId:           userID.String(),
		TenantId:         userID.String(),
		Title:            "T",
		GeneratedAt:      timestamppb.New(generatedAt),
	})
	require.NoError(t, err)
	assert.Equal(t, versionID, got.SummaryVersionID)
	assert.Equal(t, userID, got.TenantID)
	assert.True(t, generatedAt.Equal(got.GeneratedAt))
}

func TestBackfillSummaryTitleFromProto_RejectsUnparseableTenant(t *testing.T) {
	_, err := backfillSummaryTitleFromProto(&datahubv1.BackfillSummaryTitle{
		SummaryVersionId: uuid.NewString(),
		ArticleId:        uuid.NewString(),
		UserId:           uuid.NewString(),
		TenantId:         "x",
	})
	require.Error(t, err)
}

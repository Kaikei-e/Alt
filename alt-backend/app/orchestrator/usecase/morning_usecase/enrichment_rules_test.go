package morning_usecase

import (
	"fmt"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckCooldown(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	cooldown := time.Hour

	tests := []struct {
		name         string
		lastTime     time.Time
		now          time.Time
		cooldown     time.Duration
		wantCooldown bool
		wantRemain   time.Duration
	}{
		{
			name:         "zero lastTime is not in cooldown",
			lastTime:     time.Time{},
			now:          now,
			cooldown:     cooldown,
			wantCooldown: false,
			wantRemain:   0,
		},
		{
			name:         "within cooldown window",
			lastTime:     now.Add(-20 * time.Minute),
			now:          now,
			cooldown:     cooldown,
			wantCooldown: true,
			wantRemain:   40 * time.Minute,
		},
		{
			name:         "exact cooldown expired",
			lastTime:     now.Add(-time.Hour),
			now:          now,
			cooldown:     cooldown,
			wantCooldown: false,
			wantRemain:   0,
		},
		{
			name:         "well past cooldown",
			lastTime:     now.Add(-2 * time.Hour),
			now:          now,
			cooldown:     cooldown,
			wantCooldown: false,
			wantRemain:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inCooldown, remaining := checkCooldown(tt.lastTime, tt.now, tt.cooldown)
			assert.Equal(t, tt.wantCooldown, inCooldown)
			assert.Equal(t, tt.wantRemain, remaining)
		})
	}
}

func TestGroupMorningUpdates(t *testing.T) {
	feedID1 := uuid.New()
	feedID2 := uuid.New()
	unsubscribedFeedID := uuid.New()
	groupID1 := uuid.New()
	groupID2 := uuid.New()

	art1 := &domain.Article{ID: uuid.New(), FeedID: feedID1, Title: "Primary 1"}
	art1Dup := &domain.Article{ID: uuid.New(), FeedID: feedID1, Title: "Dup 1"}
	art2Unsub := &domain.Article{ID: uuid.New(), FeedID: unsubscribedFeedID, Title: "Unsub"}
	art3NoPrimary := &domain.Article{ID: uuid.New(), FeedID: feedID2, Title: "No Primary Dup"}

	groups := []*domain.MorningArticleGroup{
		{GroupID: groupID1, Article: art1, IsPrimary: true},
		{GroupID: groupID1, Article: art1Dup, IsPrimary: false},
		{GroupID: groupID2, Article: art2Unsub, IsPrimary: true},
		{GroupID: uuid.New(), Article: art3NoPrimary, IsPrimary: false},
	}

	tests := []struct {
		name        string
		groups      []*domain.MorningArticleGroup
		feedIDs     []uuid.UUID
		wantCount   int
		checkResult func(t *testing.T, updates []*domain.MorningUpdate)
	}{
		{
			name:      "empty groups gives empty updates",
			groups:    nil,
			feedIDs:   []uuid.UUID{feedID1},
			wantCount: 0,
		},
		{
			name:      "filters by subscribed feeds and primary article presence",
			groups:    groups,
			feedIDs:   []uuid.UUID{feedID1, feedID2},
			wantCount: 1,
			checkResult: func(t *testing.T, updates []*domain.MorningUpdate) {
				require.Len(t, updates, 1)
				assert.Equal(t, groupID1, updates[0].GroupID)
				assert.Equal(t, art1, updates[0].PrimaryArticle)
				require.Len(t, updates[0].Duplicates, 1)
				assert.Equal(t, art1Dup, updates[0].Duplicates[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupMorningUpdates(tt.groups, tt.feedIDs)
			assert.Len(t, got, tt.wantCount)
			if tt.checkResult != nil {
				tt.checkResult(t, got)
			}
		})
	}
}

func TestAssembleBulletEnrichments(t *testing.T) {
	artID := uuid.New()
	feedID := uuid.New()
	source := &domain.MorningLetterSourceEntry{
		SectionKey: "top",
		ArticleID:  artID,
	}
	article := &domain.Article{
		ID:      artID,
		FeedID:  feedID,
		Title:   "Go 1.26 Released",
		URL:     "https://example.com/go",
		Tags:    []string{"Go", "release"},
		Summary: "Go 1.26 brings major improvements.",
	}

	sources := []*domain.MorningLetterSourceEntry{source}
	articleByID := map[uuid.UUID]*domain.Article{artID: article}
	feedTitleByID := map[uuid.UUID]string{feedID: "Go Blog"}
	relatedByID := map[uuid.UUID][]domain.RelatedArticleTeaser{
		artID: {{ArticleID: "rel-1", Title: "Go 1.25"}},
	}
	relatedMetaByID := map[string]*domain.Article{}

	got := assembleBulletEnrichments(sources, articleByID, feedTitleByID, relatedByID, relatedMetaByID)
	require.Len(t, got, 1)
	assert.Equal(t, "top", got[0].SectionKey)
	assert.Equal(t, artID.String(), got[0].ArticleID)
	assert.Equal(t, "Go 1.26 Released", got[0].ArticleTitle)
	assert.Equal(t, "https://example.com/go", got[0].ArticleURL)
	assert.Equal(t, "Go Blog", got[0].FeedTitle)
	assert.NotEmpty(t, got[0].ArticleAltHref)
	assert.NotEmpty(t, got[0].ChatHref)

	// Missing article fallback
	missingSources := []*domain.MorningLetterSourceEntry{{SectionKey: "misc", ArticleID: uuid.New()}}
	fallback := assembleBulletEnrichments(missingSources, map[uuid.UUID]*domain.Article{}, nil, nil, nil)
	require.Len(t, fallback, 1)
	assert.Equal(t, fmt.Sprintf("/articles/%s", missingSources[0].ArticleID.String()), fallback[0].ArticleAltHref)
}

func TestEnrichRelatedTeasers(t *testing.T) {
	id := uuid.New().String()
	art := &domain.Article{
		ID:    uuid.MustParse(id),
		Title: "Found Title",
		URL:   "https://example.com/item",
	}

	tests := []struct {
		name     string
		teasers  []domain.RelatedArticleTeaser
		meta     map[string]*domain.Article
		expected string
	}{
		{
			name:     "empty teasers",
			teasers:  nil,
			meta:     nil,
			expected: "",
		},
		{
			name: "hydrates metadata when available",
			teasers: []domain.RelatedArticleTeaser{
				{ArticleID: id, Title: ""},
			},
			meta:     map[string]*domain.Article{id: art},
			expected: "Found Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := enrichRelatedTeasers(tt.teasers, tt.meta)
			if len(tt.teasers) > 0 {
				assert.Equal(t, tt.expected, res[0].Title)
			}
		})
	}
}

func TestBuildArticleAltHref(t *testing.T) {
	tests := []struct {
		name       string
		id, url, t string
		want       string
	}{
		{name: "empty id", id: "", url: "x", t: "y", want: ""},
		{name: "bare id", id: "123", url: "", t: "", want: "/articles/123"},
		{name: "with query", id: "123", url: "https://a.com", t: "Title", want: "/articles/123?title=Title&url=https%3A%2F%2Fa.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildArticleAltHref(tt.id, tt.url, tt.t))
		})
	}
}

func TestBuildChatHref(t *testing.T) {
	assert.Empty(t, buildChatHref("", "title"))
	assert.Equal(t, "/augur?articleId=123&context=Test", buildChatHref("123", "Test"))
}

func TestBuildExcerpt(t *testing.T) {
	art := &domain.Article{Summary: "Short summary"}
	assert.Equal(t, "Short summary", buildExcerpt(art))

	art2 := &domain.Article{Content: "Only content available"}
	assert.Equal(t, "Only content available", buildExcerpt(art2))

	assert.Empty(t, buildExcerpt(&domain.Article{}))
}

func TestCollapseWhitespace(t *testing.T) {
	assert.Equal(t, "hello world", collapseWhitespace("  hello \n \t world  "))
}

func TestNormalizeTags(t *testing.T) {
	input := []string{"Go", "go", " Rust ", "", "PYTHON"}
	got := normalizeTags(input)
	assert.Equal(t, []string{"Go", "Rust", "PYTHON"}, got)
}

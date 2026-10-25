package driver

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

func TestContainsCJK_Japanese(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"イラン 石油", true},
		{"石油危機 原因", true},
		{"量子コンピュータ 実用化", true},
		{"Iran oil crisis", false},
		{"hello world", false},
		{"", false},
		{"AI関連ニュース", true},
		{"GPT-4o と Claude", true},        // と is Hiragana → CJK
		{"GPT-4oとClaude 3.5の違いは？", true}, // contains と, の, は (Hiragana)
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := containsCJK(tt.input)
			if got != tt.expected {
				t.Errorf("containsCJK(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBuildUserFilter(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		expected string
	}{
		{
			name:     "valid userID",
			userID:   "user-123",
			expected: `user_id = "user-123"`,
		},
		{
			name:     "escaping quotes and backslashes",
			userID:   `user"1\2`,
			expected: `user_id = "user\"1\\2"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildUserFilter(tt.userID)
			if result != tt.expected {
				t.Errorf("BuildUserFilter(%q) = %q, want %q", tt.userID, result, tt.expected)
			}
		})
	}

	t.Run("empty userID panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("BuildUserFilter with empty string should panic")
			}
		}()
		_ = BuildUserFilter("")
	})

	t.Run("whitespace userID panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("BuildUserFilter with whitespace string should panic")
			}
		}()
		_ = BuildUserFilter("   ")
	})
}

func TestMeilisearchDriver_SearchByUserID_RejectsEmptyUserID(t *testing.T) {
	driver := &MeilisearchDriver{}

	_, err := driver.SearchByUserID(t.Context(), "query", "", 10)
	if err == nil {
		t.Error("SearchByUserID with empty userID should return error")
	}

	_, err = driver.SearchByUserID(t.Context(), "query", "   ", 10)
	if err == nil {
		t.Error("SearchByUserID with whitespace userID should return error")
	}
}

func TestMeilisearchDriver_SearchByUserIDWithPagination_RejectsEmptyUserID(t *testing.T) {
	driver := &MeilisearchDriver{}

	_, _, err := driver.SearchByUserIDWithPagination(t.Context(), "query", "", 0, 10)
	if err == nil {
		t.Error("SearchByUserIDWithPagination with empty userID should return error")
	}

	_, _, err = driver.SearchByUserIDWithPagination(t.Context(), "query", "   ", 0, 10)
	if err == nil {
		t.Error("SearchByUserIDWithPagination with whitespace userID should return error")
	}
}

func TestBuildUserDateFilter(t *testing.T) {
	after := time.Unix(1700000000, 0)
	before := time.Unix(1700003600, 0)

	tests := []struct {
		name     string
		userID   string
		after    *time.Time
		before   *time.Time
		expected string
	}{
		{
			name:     "both bounds present",
			userID:   "u1",
			after:    &after,
			before:   &before,
			expected: `user_id = "u1" AND published_at >= 1700000000 AND published_at <= 1700003600`,
		},
		{
			name:     "only published_after",
			userID:   "u1",
			after:    &after,
			before:   nil,
			expected: `user_id = "u1" AND published_at >= 1700000000`,
		},
		{
			name:     "only published_before",
			userID:   "u1",
			after:    nil,
			before:   &before,
			expected: `user_id = "u1" AND published_at <= 1700003600`,
		},
		{
			name:     "neither bound present",
			userID:   "u1",
			after:    nil,
			before:   nil,
			expected: `user_id = "u1"`,
		},
		{
			name:     "escapes user ID with quotes",
			userID:   `user"1`,
			after:    &after,
			before:   &before,
			expected: `user_id = "user\"1" AND published_at >= 1700000000 AND published_at <= 1700003600`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildUserDateFilter(tt.userID, tt.after, tt.before)
			if got != tt.expected {
				t.Errorf("BuildUserDateFilter() = %q, want %q", got, tt.expected)
			}
		})
	}

	t.Run("empty userID panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("BuildUserDateFilter with empty userID should panic")
			}
		}()
		_ = BuildUserDateFilter("", &after, &before)
	})

	t.Run("whitespace userID panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("BuildUserDateFilter with whitespace userID should panic")
			}
		}()
		_ = BuildUserDateFilter("   ", &after, &before)
	})
}

func TestMeilisearchDriver_SearchByUserIDWithDateFilter_RejectsEmptyUserID(t *testing.T) {
	driver := &MeilisearchDriver{}
	after := time.Unix(1700000000, 0)

	_, err := driver.SearchByUserIDWithDateFilter(t.Context(), "query", "", &after, nil, 10)
	if err == nil {
		t.Error("SearchByUserIDWithDateFilter with empty userID should return error")
	}

	_, err = driver.SearchByUserIDWithDateFilter(t.Context(), "query", "   ", &after, nil, 10)
	if err == nil {
		t.Error("SearchByUserIDWithDateFilter with whitespace userID should return error")
	}
}

func TestSanitizeFilterValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain value",
			input:    "user-123",
			expected: "user-123",
		},
		{
			name:     "escapes backslash, double quote, and single quote",
			input:    "a\\b\"c\x27d",
			expected: "a\\\\b\\\"c\\\x27d",
		},
		{
			name:     "strips ASCII control chars and DEL",
			input:    "u\x00s\ne\rr\t\x1f1\x7f2",
			expected: "user12",
		},
		{
			name:     "filter injection attempt is neutralized",
			input:    `u1" OR user_id != "u2`,
			expected: `u1\" OR user_id != \"u2`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFilterValue(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeFilterValue(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMakeSecureSearchFilter(t *testing.T) {
	tests := []struct {
		name     string
		tags     []string
		expected string
	}{
		{
			name:     "empty tags",
			tags:     nil,
			expected: "",
		},
		{
			name:     "single tag is quoted",
			tags:     []string{"golang"},
			expected: `tags = "golang"`,
		},
		{
			name:     "multiple tags with injection characters are quoted and escaped",
			tags:     []string{`go" OR 1=1`, "sec\\ops\x27test\x00"},
			expected: "tags = \"go\\\" OR 1=1\" AND tags = \"sec\\\\ops\\\x27test\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeSecureSearchFilter(tt.tags)
			if got != tt.expected {
				t.Errorf("makeSecureSearchFilter(%v) = %q, want %q", tt.tags, got, tt.expected)
			}
		})
	}
}

func TestMeilisearchDriver_EnsureIndex_CombinesJpnAndEngLocalizedAttributes(t *testing.T) {
	fake := newFakeIndexManager()
	sm := &fakeServiceManager{idx: fake}
	d := NewMeilisearchDriverWithClients(sm, nil, "articles")

	if err := d.EnsureIndex(context.Background()); err != nil {
		t.Fatalf("EnsureIndex() error = %v", err)
	}

	if len(fake.localizedAttrs) != 1 {
		t.Fatalf("localizedAttrs rules = %d, want 1 combined rule (got %+v)", len(fake.localizedAttrs), fake.localizedAttrs)
	}
	wantLocales := []string{"jpn", "eng"}
	if !reflect.DeepEqual(fake.localizedAttrs[0].Locales, wantLocales) {
		t.Errorf("Locales = %v, want %v", fake.localizedAttrs[0].Locales, wantLocales)
	}
	wantPatterns := []string{"title", "content", "tags"}
	if !reflect.DeepEqual(fake.localizedAttrs[0].AttributePatterns, wantPatterns) {
		t.Errorf("AttributePatterns = %v, want %v", fake.localizedAttrs[0].AttributePatterns, wantPatterns)
	}
}

func TestMeilisearchDriver_SearchMethods_DoNotOverrideLocalesOnCJKQuery(t *testing.T) {
	cjkQuery := "GPT-4o と Claude ゼレンスキー"

	t.Run("SearchByUserID", func(t *testing.T) {
		fake := newFakeIndexManager()
		sm := &fakeServiceManager{idx: fake}
		d := NewMeilisearchDriverWithClients(sm, nil, "articles")

		if _, err := d.SearchByUserID(context.Background(), cjkQuery, "u1", 10); err != nil {
			t.Fatalf("SearchByUserID() error = %v", err)
		}
		if len(fake.lastSearchReq.Locales) != 0 {
			t.Errorf("SearchByUserID Locales = %v, want empty so Meilisearch uses all configured index locales", fake.lastSearchReq.Locales)
		}
	})

	t.Run("SearchByUserIDWithPagination", func(t *testing.T) {
		fake := newFakeIndexManager()
		sm := &fakeServiceManager{idx: fake}
		d := NewMeilisearchDriverWithClients(sm, nil, "articles")

		if _, _, err := d.SearchByUserIDWithPagination(context.Background(), cjkQuery, "u1", 0, 20); err != nil {
			t.Fatalf("SearchByUserIDWithPagination() error = %v", err)
		}
		if len(fake.lastSearchReq.Locales) != 0 {
			t.Errorf("SearchByUserIDWithPagination Locales = %v, want empty so Meilisearch uses all configured index locales", fake.lastSearchReq.Locales)
		}
	})

	t.Run("SearchByUserIDWithDateFilter", func(t *testing.T) {
		fake := newFakeIndexManager()
		sm := &fakeServiceManager{idx: fake}
		d := NewMeilisearchDriverWithClients(sm, nil, "articles")
		after := time.Unix(1700000000, 0)

		if _, err := d.SearchByUserIDWithDateFilter(context.Background(), cjkQuery, "u1", &after, nil, 10); err != nil {
			t.Fatalf("SearchByUserIDWithDateFilter() error = %v", err)
		}
		if len(fake.lastSearchReq.Locales) != 0 {
			t.Errorf("SearchByUserIDWithDateFilter Locales = %v, want empty so Meilisearch uses all configured index locales", fake.lastSearchReq.Locales)
		}
	})
}

func TestMeilisearchDriver_FiltersLowScorePureVectorHits_PreservesBM25Hits_AndClampsEstimatedTotal(t *testing.T) {
	fake := newFakeIndexManager()
	// Simulate hybrid search response for a query like "小浜" or "ゼレンスキー":
	// - hit 1: BM25 content-only hit with _rankingScore = 0.7020 (< 0.75, MUST be kept because "words" is present)
	// - hit 2: BM25 content-only hit with _rankingScore = 0.6869 (< 0.75, MUST be kept because "words" is present)
	// - hit 3: Pure vector hit with _rankingScore = 0.8100 (>= 0.75, MUST be kept)
	// - hit 4: Pure vector noise hit with _rankingScore = 0.7100 (< 0.75, MUST be dropped)
	// - hit 5: Pure vector noise hit with _rankingScore = 0.6850 (< 0.75, MUST be dropped)
	fake.searchResp = &meilisearch.SearchResponse{
		Hits: meilisearch.Hits{
			makeHit(t, map[string]any{
				"id":            "bm25-1",
				"title":         "Article 1",
				"_rankingScore": 0.7020,
				"_rankingScoreDetails": map[string]any{
					"words": map[string]any{"order": 0, "matchingWords": 1, "maxMatchingWords": 1, "score": 1.0},
				},
			}),
			makeHit(t, map[string]any{
				"id":            "bm25-2",
				"title":         "Article 2",
				"_rankingScore": 0.6869,
				"_rankingScoreDetails": map[string]any{
					"words":      map[string]any{"order": 0, "matchingWords": 1, "maxMatchingWords": 1, "score": 1.0},
					"vectorSort": map[string]any{"order": 1, "similarity": 0.6869},
				},
			}),
			makeHit(t, map[string]any{
				"id":            "vec-high",
				"title":         "Semantic Match",
				"_rankingScore": 0.8100,
				"_rankingScoreDetails": map[string]any{
					"vectorSort": map[string]any{"order": 0, "similarity": 0.8100},
				},
			}),
			makeHit(t, map[string]any{
				"id":            "vec-noise-1",
				"title":         "Unrelated 1",
				"_rankingScore": 0.7100,
				"_rankingScoreDetails": map[string]any{
					"vectorSort": map[string]any{"order": 0, "similarity": 0.7100},
				},
			}),
			makeHit(t, map[string]any{
				"id":            "vec-noise-2",
				"title":         "Unrelated 2",
				"_rankingScore": 0.6850,
				"_rankingScoreDetails": map[string]any{
					"vectorSort": map[string]any{"order": 0, "similarity": 0.6850},
				},
			}),
		},
		EstimatedTotalHits: 1000,
		ProcessingTimeMs:   12,
	}

	sm := &fakeServiceManager{idx: fake}
	d := NewMeilisearchDriverWithClients(sm, nil, "articles").WithHybrid(&HybridConfig{
		Embedder:         "bge-m3",
		SemanticRatio:    0.3,
		MinSemanticScore: 0.75,
	})

	docs, total, err := d.SearchByUserIDWithPagination(context.Background(), "小浜", "u1", 0, 20)
	if err != nil {
		t.Fatalf("SearchByUserIDWithPagination() error = %v", err)
	}

	if len(docs) != 3 {
		t.Fatalf("len(docs) = %d, want 3 (2 BM25 + 1 high-score vector); got %+v", len(docs), docs)
	}
	wantIDs := []string{"bm25-1", "bm25-2", "vec-high"}
	for i, wantID := range wantIDs {
		if docs[i].ID != wantID {
			t.Errorf("docs[%d].ID = %q, want %q", i, docs[i].ID, wantID)
		}
	}
	if total != 3 {
		t.Errorf("estimatedTotal = %d, want 3 (clamped from 1000 because pure vector tail was filtered)", total)
	}
}

func TestMeilisearchDriver_AllPureVectorNoiseFiltered_ClampsEstimatedTotalToZero(t *testing.T) {
	fake := newFakeIndexManager()
	fake.searchResp = &meilisearch.SearchResponse{
		Hits: meilisearch.Hits{
			makeHit(t, map[string]any{
				"id":            "vec-noise-1",
				"title":         "Unrelated 1",
				"_rankingScore": 0.695,
				"_rankingScoreDetails": map[string]any{
					"vectorSort": map[string]any{"order": 0, "similarity": 0.695},
				},
			}),
		},
		EstimatedTotalHits: 1000,
	}

	sm := &fakeServiceManager{idx: fake}
	d := NewMeilisearchDriverWithClients(sm, nil, "articles").WithHybrid(&HybridConfig{
		Embedder:         "bge-m3",
		SemanticRatio:    0.3,
		MinSemanticScore: 0.75,
	})

	docs, total, err := d.SearchByUserIDWithPagination(context.Background(), "nonexistent", "u1", 0, 20)
	if err != nil {
		t.Fatalf("SearchByUserIDWithPagination() error = %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("len(docs) = %d, want 0", len(docs))
	}
	if total != 0 {
		t.Errorf("estimatedTotal = %d, want 0", total)
	}
}

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rag-orchestrator/eval"
)

func TestArticleScopePattern_SplitsScopeFromQuestion(t *testing.T) {
	// The envelope the chat handler builds around an article-scoped question.
	// Both title and question are synthetic here.
	content := "Regarding the article: Synthetic Title | Example Wire [articleId: 11111111-2222-4333-8444-555555555555]\n\nQuestion:\nこの記事の要点を3点にまとめて"

	m := articleScopePattern.FindStringSubmatch(content)
	require.Len(t, m, 4)
	assert.Equal(t, "Synthetic Title | Example Wire", m[1])
	assert.Equal(t, "11111111-2222-4333-8444-555555555555", m[2])
	assert.Equal(t, "この記事の要点を3点にまとめて", m[3])
}

func TestArticleScopePattern_IgnoresPlainQuery(t *testing.T) {
	assert.Nil(t, articleScopePattern.FindStringSubmatch("ふつうの質問です"))
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "hiragana", text: "この記事について", want: "ja"},
		{name: "katakana", text: "エージェント", want: "ja"},
		{name: "kanji only", text: "量子計算", want: "ja"},
		{name: "latin", text: "How does this work?", want: "en"},
		{name: "latin with digits", text: "Runtime 1.27 release notes", want: "en"},
		{name: "empty", text: "", want: "en"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, detectLanguage(tt.text))
		})
	}
}

func TestSummarizeLanguages(t *testing.T) {
	assert.Equal(t, "", summarizeLanguages(map[string]bool{}))
	assert.Equal(t, "ja", summarizeLanguages(map[string]bool{"ja": true}))
	assert.Equal(t, "en", summarizeLanguages(map[string]bool{"en": true}))
	assert.Equal(t, eval.LanguageMixed, summarizeLanguages(map[string]bool{"ja": true, "en": true}))
}

func TestDeriveCategory(t *testing.T) {
	scoped := eval.GoldenCase{ArticleScope: &eval.ArticleScopeInfo{ArticleID: "a", Title: "t"}}
	followUp := eval.GoldenCase{ConversationHistory: []eval.HistoryMessage{{Role: "user", Content: "q"}}}
	crossLingual := eval.GoldenCase{Language: eval.LanguagePair{Query: "ja", Corpus: "en"}}

	assert.Equal(t, eval.CategoryNoAnswer, deriveCategory(scoped, 0), "no resolved article outranks every other signal")
	assert.Equal(t, eval.CategoryArticleScoped, deriveCategory(scoped, 1))
	assert.Equal(t, eval.CategoryFollowUp, deriveCategory(followUp, 2))
	assert.Equal(t, eval.CategoryCrossLingual, deriveCategory(crossLingual, 2))
	assert.Equal(t, eval.CategoryRecallMiss, deriveCategory(eval.GoldenCase{}, 2))
}

func TestDriftCandidates(t *testing.T) {
	drift := driftCandidates([]string{"a", "b", "c"}, []string{"b"})
	assert.Equal(t, []string{"a", "c"}, drift)
	assert.Empty(t, driftCandidates(nil, []string{"b"}))
	assert.Empty(t, driftCandidates([]string{"b"}, []string{"b"}))
}

func TestDedupeExcluding(t *testing.T) {
	got := dedupeExcluding([]string{"a", "b", "a", "", "c"}, []string{"c"})
	assert.Equal(t, []string{"a", "b"}, got)
	assert.Empty(t, dedupeExcluding(nil, nil))
}

func TestRetrievalScope(t *testing.T) {
	assert.Equal(t, "article_only", retrievalScope(eval.GoldenCase{ArticleScope: &eval.ArticleScopeInfo{ArticleID: "a"}}))
	assert.Equal(t, "global", retrievalScope(eval.GoldenCase{}))
}

func TestGenerateConfig_Validate(t *testing.T) {
	valid := generateConfig{
		DSN:            "postgres://user@localhost:5436/rag_db",
		OutputPath:     "eval/testdata/golden_cases.local.json",
		Since:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		MaxCases:       60,
		RelevantPerCas: 5,
	}

	tests := []struct {
		name    string
		mutate  func(*generateConfig)
		wantErr string
	}{
		{name: "valid", mutate: func(*generateConfig) {}},
		{name: "missing dsn", mutate: func(c *generateConfig) { c.DSN = "" }, wantErr: ragDBDSNEnv},
		{name: "missing output", mutate: func(c *generateConfig) { c.OutputPath = "" }, wantErr: "output path"},
		{name: "zero max cases", mutate: func(c *generateConfig) { c.MaxCases = 0 }, wantErr: "max cases"},
		{name: "zero relevant per case", mutate: func(c *generateConfig) { c.RelevantPerCas = 0 }, wantErr: "relevant-per-case"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)
			err := cfg.validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

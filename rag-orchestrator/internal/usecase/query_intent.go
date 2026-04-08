package usecase

import (
	"errors"
	"strings"

	"rag-orchestrator/internal/domain"
)

// IntentType classifies the type of user query.
type IntentType string

const (
	IntentGeneral           IntentType = "general"
	IntentArticleScoped     IntentType = "article_scoped"
	IntentComparison        IntentType = "comparison"
	IntentTemporal          IntentType = "temporal"
	IntentTopicDeepDive     IntentType = "topic_deep_dive"
	IntentFactCheck         IntentType = "fact_check"
	IntentCausalExplanation IntentType = "causal_explanation"
	IntentSynthesis         IntentType = "synthesis"
)

// SubIntentType classifies the analytical intent within an article-scoped query.
// Separate type from IntentType to prevent accidental use as strategy map key.
type SubIntentType string

const (
	SubIntentNone            SubIntentType = ""
	SubIntentCritique        SubIntentType = "critique"         // 反論・批判・弱点
	SubIntentOpinion         SubIntentType = "opinion"          // 評価・意見
	SubIntentImplication     SubIntentType = "implication"      // 影響・意味合い
	SubIntentDetail          SubIntentType = "detail"           // 技術的・詳細・具体例
	SubIntentRelatedArticles SubIntentType = "related_articles" // 関連する記事
	SubIntentEvidence        SubIntentType = "evidence"         // 根拠・エビデンス
	SubIntentSummaryRefresh  SubIntentType = "summary_refresh"  // 要約リフレッシュ
)

// ErrArticleNotIndexed indicates that the referenced article is not in the RAG index.
var ErrArticleNotIndexed = errors.New("article not indexed in RAG system")

// QueryIntent holds the parsed intent from a raw user query.
type QueryIntent struct {
	IntentType    IntentType
	SubIntentType SubIntentType // Analytical sub-intent for article-scoped queries
	ArticleID     string
	ArticleTitle  string
	UserQuestion  string   // Metadata-stripped question body
	OriginalQuery string
	SearchQueries []string // Planner-generated search queries (topic-aware)
}

// ParseQueryIntent parses a raw query to determine intent and extract metadata.
// Uses step-based parsing (not regex) to handle edge cases like brackets in titles.
func ParseQueryIntent(rawQuery string) QueryIntent {
	intent := QueryIntent{
		IntentType:    IntentGeneral,
		OriginalQuery: rawQuery,
		UserQuestion:  rawQuery,
	}

	// Step 1: prefix check
	const prefix = "Regarding the article: "
	if !strings.HasPrefix(rawQuery, prefix) {
		return intent
	}

	// Step 2: split at last "\n\nQuestion:\n" occurrence
	const sep = "\n\nQuestion:\n"
	sepIdx := strings.LastIndex(rawQuery, sep)
	if sepIdx < 0 {
		return intent
	}
	headerPart := rawQuery[len(prefix):sepIdx]
	intent.UserQuestion = strings.TrimSpace(rawQuery[sepIdx+len(sep):])

	// Step 3: detect "[articleId: ...]" from the end of header
	const artPrefix = "[articleId: "
	artStart := strings.LastIndex(headerPart, artPrefix)
	if artStart < 0 {
		return intent
	}
	artEnd := strings.Index(headerPart[artStart:], "]")
	if artEnd < 0 {
		return intent
	}
	intent.ArticleID = strings.TrimSpace(headerPart[artStart+len(artPrefix) : artStart+artEnd])
	intent.ArticleTitle = strings.TrimSpace(headerPart[:artStart])
	intent.IntentType = IntentArticleScoped
	return intent
}

// ResolveQueryIntent resolves article scope from the current query first, then
// falls back to the most recent article-scoped user message in conversation history.
func ResolveQueryIntent(rawQuery string, history []domain.Message) QueryIntent {
	current := ParseQueryIntent(rawQuery)
	if current.IntentType == IntentArticleScoped {
		return current
	}

	trimmedQuery := strings.TrimSpace(rawQuery)
	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]
		if msg.Role != "user" {
			continue
		}
		prev := ParseQueryIntent(msg.Content)
		if prev.IntentType != IntentArticleScoped {
			continue
		}
		prev.OriginalQuery = rawQuery
		prev.UserQuestion = trimmedQuery
		return prev
	}

	return current
}

package domain

import "strings"

// Known placeholder summary texts persisted by pre-processor when an article
// cannot be summarized (too short, too long, dead-lettered, or exhausted quality
// retries). These mark the article as processed in article_summaries so workers
// stop retrying, but must never be promoted into summary_versions or emitted as
// SummaryVersionCreated (ADR-001014, ADR-001042).
const (
	PlaceholderSummaryTooShort            = "本文が短すぎるため要約できませんでした。"
	PlaceholderSummaryTooLong             = "本文が長すぎるため要約できませんでした。"
	PlaceholderSummaryDeadLetter          = "本文をモデルが処理できなかったため要約できませんでした。"
	PlaceholderSummaryLowQualityExhausted = "品質基準を満たす要約を生成できなかったため要約できませんでした。"
)

// IsPlaceholderSummary reports whether summary is a known non-summary
// placeholder message that must not be versioned or projected as ready.
func IsPlaceholderSummary(summary string) bool {
	switch strings.TrimSpace(summary) {
	case PlaceholderSummaryTooShort,
		PlaceholderSummaryTooLong,
		PlaceholderSummaryDeadLetter,
		PlaceholderSummaryLowQualityExhausted:
		return true
	default:
		return false
	}
}

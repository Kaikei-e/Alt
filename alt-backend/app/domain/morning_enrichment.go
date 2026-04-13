package domain

// MorningLetterBulletEnrichment is a rendered-view augmentation for a
// single Morning Letter source. One entry per (section_key, article_id)
// tuple taken from morning_letter_sources. All fields are optional —
// absence means "Alt has no richer information available yet", which is
// a normal state early in an article's lifecycle.
type MorningLetterBulletEnrichment struct {
	SectionKey   string
	ArticleID    string
	ArticleTitle string
	ArticleURL   string
	// In-Alt route, e.g. "/articles/<uuid>".
	ArticleAltHref string
	FeedTitle      string
	// Tag chips. Empty when tag generation has not caught up.
	Tags []string
	// Up to ~3 related-article teasers from the search-indexer.
	RelatedArticles []RelatedArticleTeaser
	// Optional short summary excerpt (1-3 sentences) drawn from the
	// article's stored summary or first content sentences.
	SummaryExcerpt string
	// Deep-link into Acolyte, pre-seeded with article context.
	AcolyteHref string
}

// RelatedArticleTeaser is the minimum shape needed to render a related
// article pill that can be clicked to navigate into Alt.
type RelatedArticleTeaser struct {
	ArticleID      string
	Title          string
	ArticleAltHref string
	FeedTitle      string
}

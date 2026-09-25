package morning_usecase

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"alt/domain"

	"github.com/google/uuid"
)

// checkCooldown evaluates whether the given lastTime is within the cooldown window.
func checkCooldown(lastTime, now time.Time, cooldown time.Duration) (bool, time.Duration) {
	if lastTime.IsZero() {
		return false, 0
	}
	diff := now.Sub(lastTime)
	if diff < cooldown {
		return true, cooldown - diff
	}
	return false, 0
}

// groupMorningUpdates filters article groups by subscribed feeds, collects updates by group, and keeps those with a primary article.
func groupMorningUpdates(groups []*domain.MorningArticleGroup, feedIDs []uuid.UUID) []*domain.MorningUpdate {
	feedIDMap := make(map[uuid.UUID]bool, len(feedIDs))
	for _, feedID := range feedIDs {
		feedIDMap[feedID] = true
	}

	groupedMap := make(map[string]*domain.MorningUpdate)
	for _, g := range groups {
		if g.Article == nil || !feedIDMap[g.Article.FeedID] {
			continue
		}

		groupIDStr := g.GroupID.String()
		update, exists := groupedMap[groupIDStr]
		if !exists {
			update = &domain.MorningUpdate{
				GroupID:    g.GroupID,
				Duplicates: []*domain.Article{},
			}
			groupedMap[groupIDStr] = update
		}

		if g.IsPrimary {
			update.PrimaryArticle = g.Article
		} else {
			update.Duplicates = append(update.Duplicates, g.Article)
		}
	}

	var updates []*domain.MorningUpdate
	for _, update := range groupedMap {
		if update.PrimaryArticle != nil {
			updates = append(updates, update)
		}
	}

	return updates
}

// assembleBulletEnrichments constructs bullet enrichments for each source entry preserving source order.
func assembleBulletEnrichments(
	sources []*domain.MorningLetterSourceEntry,
	articleByID map[uuid.UUID]*domain.Article,
	feedTitleByID map[uuid.UUID]string,
	relatedByID map[uuid.UUID][]domain.RelatedArticleTeaser,
	relatedMetaByID map[string]*domain.Article,
) []*domain.MorningLetterBulletEnrichment {
	out := make([]*domain.MorningLetterBulletEnrichment, 0, len(sources))
	for _, s := range sources {
		article := articleByID[s.ArticleID]
		enrichment := &domain.MorningLetterBulletEnrichment{
			SectionKey: s.SectionKey,
			ArticleID:  s.ArticleID.String(),
		}
		if article != nil {
			enrichment.ArticleTitle = article.Title
			enrichment.ArticleURL = article.URL
			enrichment.ArticleAltHref = buildArticleAltHref(article.ID.String(), article.URL, article.Title)
			enrichment.Tags = normalizeTags(article.Tags)
			enrichment.SummaryExcerpt = buildExcerpt(article)
			enrichment.FeedTitle = feedTitleByID[article.FeedID]
			enrichment.ChatHref = buildChatHref(article.ID.String(), article.Title)
			enrichment.RelatedArticles = enrichRelatedTeasers(relatedByID[article.ID], relatedMetaByID)
		} else {
			// Graceful degradation: bare href so the card still renders.
			enrichment.ArticleAltHref = fmt.Sprintf("/articles/%s", s.ArticleID.String())
		}
		out = append(out, enrichment)
	}
	return out
}

// enrichRelatedTeasers rewrites each teaser's ArticleAltHref to include
// ?url&title when DB metadata is available, leaving the bare /articles/<id>
// form otherwise.
func enrichRelatedTeasers(
	teasers []domain.RelatedArticleTeaser,
	metaByID map[string]*domain.Article,
) []domain.RelatedArticleTeaser {
	if len(teasers) == 0 {
		return teasers
	}
	out := make([]domain.RelatedArticleTeaser, len(teasers))
	for i, t := range teasers {
		out[i] = t
		if a, ok := metaByID[t.ArticleID]; ok {
			out[i].ArticleAltHref = buildArticleAltHref(a.ID.String(), a.URL, a.Title)
			if t.Title == "" {
				out[i].Title = a.Title
			}
		}
	}
	return out
}

// buildArticleAltHref produces the in-Alt route for an article card.
// When url/title are present the route gains ?url&title query params so
// /articles/[id]/+page.svelte can fetch content on-the-fly. When the
// article metadata is missing the bare /articles/<id> form is returned
// (graceful-degradation invariant from ADR-000707).
func buildArticleAltHref(articleID, articleURL, title string) string {
	if articleID == "" {
		return ""
	}
	base := "/articles/" + articleID
	if articleURL == "" && title == "" {
		return base
	}
	v := url.Values{}
	if articleURL != "" {
		v.Set("url", articleURL)
	}
	if title != "" {
		v.Set("title", title)
	}
	return base + "?" + v.Encode()
}

// buildChatHref pre-seeds Augur (the Alt chat surface) with the article
// id and title so the user can ask a follow-up. The query keys match
// resolveAugurEntry in the frontend.
func buildChatHref(articleID, title string) string {
	if articleID == "" {
		return ""
	}
	v := url.Values{}
	v.Set("articleId", articleID)
	if title != "" {
		v.Set("context", title)
	}
	return "/augur?" + v.Encode()
}

// buildExcerpt prefers the stored summary; falls back to the first
// sentences of content. Trims to summaryExcerptChars.
func buildExcerpt(a *domain.Article) string {
	src := strings.TrimSpace(a.Summary)
	if src == "" {
		src = strings.TrimSpace(a.Content)
	}
	if src == "" {
		return ""
	}
	src = collapseWhitespace(src)
	if len(src) <= summaryExcerptChars {
		return src
	}
	truncated := src[:summaryExcerptChars]
	if i := strings.LastIndex(truncated, " "); i > summaryExcerptChars/2 {
		truncated = truncated[:i]
	}
	return truncated + "…"
}

// collapseWhitespace replaces runs of whitespace with a single space so
// excerpts stay single-line in card layouts.
func collapseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == '\n' || r == '\t' || r == '\r' || r == ' ' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}

func normalizeTags(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, dup := seen[strings.ToLower(t)]; dup {
			continue
		}
		seen[strings.ToLower(t)] = struct{}{}
		out = append(out, t)
	}
	return out
}

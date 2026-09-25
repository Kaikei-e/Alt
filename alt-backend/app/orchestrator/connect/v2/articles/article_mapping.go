package articles

import (
	"net/url"
	"time"

	"alt/domain"
	articlesv2 "alt/gen/proto/alt/articles/v2"
	"alt/utils/safeconv"
)

// clampPageLimit bounds limit to [1, max] and falls back to def if non-positive.
func clampPageLimit(limit int32, def, max int) int {
	if limit <= 0 {
		return def
	}
	val := int(limit)
	if val > max {
		return max
	}
	return val
}

// deriveRFC3339NanoCursor formats a pagination cursor with nanosecond precision when more items exist.
func deriveRFC3339NanoCursor(publishedAt time.Time, hasMore bool) *string {
	if !hasMore {
		return nil
	}
	cursorStr := publishedAt.Format(time.RFC3339Nano)
	return &cursorStr
}

// prefetchTargetRejection records a rejected raw URL and why it was dropped.
type prefetchTargetRejection struct {
	RawURL string
	Reason string
	Err    error
}

// dedupeValidPrefetchTargets validates URLs, limits batch size, and keeps at most one URL per host.
func dedupeValidPrefetchTargets(
	rawURLs []string,
	maxURLs int,
	validate func(*url.URL) error,
) ([]*url.URL, []prefetchTargetRejection, int) {
	if len(rawURLs) > maxURLs {
		rawURLs = rawURLs[:maxURLs]
	}

	valid := make([]*url.URL, 0, len(rawURLs))
	var rejections []prefetchTargetRejection
	claimedHosts := make(map[string]struct{}, len(rawURLs))
	skippedSameHost := 0

	for _, raw := range rawURLs {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil {
			rejections = append(rejections, prefetchTargetRejection{
				RawURL: raw,
				Reason: "unparseable",
				Err:    parseErr,
			})
			continue
		}

		// The same SSRF allowlist FetchArticleContent applies. A batch path
		// that skipped it would be a way to reach the loopback and metadata
		// addresses the single-URL path refuses.
		if allowErr := validate(parsed); allowErr != nil {
			rejections = append(rejections, prefetchTargetRejection{
				RawURL: raw,
				Reason: "not allowed",
				Err:    allowErr,
			})
			continue
		}

		// One entry per host, decided before anything is claimed. A second
		// URL on a host whose turn this batch already took could only be
		// refused by the crawl-delay gate — after that refusal had cost the
		// gate an ask, and the ask is what reserves the window.
		host := parsed.Host
		if _, taken := claimedHosts[host]; taken {
			skippedSameHost++
			continue
		}
		claimedHosts[host] = struct{}{}
		valid = append(valid, parsed)
	}

	return valid, rejections, skippedSameHost
}

// convertArticlesToProto converts domain articles to proto format.
func convertArticlesToProto(articles []*domain.Article) []*articlesv2.ArticleItem {
	result := make([]*articlesv2.ArticleItem, 0, len(articles))
	for _, article := range articles {
		result = append(result, &articlesv2.ArticleItem{
			Id:          article.ID.String(),
			Title:       article.Title,
			Url:         article.URL,
			Content:     article.Content,
			PublishedAt: article.PublishedAt.Format(time.RFC3339),
			Tags:        article.Tags,
		})
	}
	return result
}

// convertTagTrailArticlesToProto converts domain tag trail articles to proto format.
func convertTagTrailArticlesToProto(articles []*domain.TagTrailArticle) []*articlesv2.TagTrailArticleItem {
	result := make([]*articlesv2.TagTrailArticleItem, 0, len(articles))
	for _, article := range articles {
		result = append(result, &articlesv2.TagTrailArticleItem{
			Id:          article.ID,
			Title:       article.Title,
			Link:        article.Link,
			PublishedAt: article.PublishedAt.Format(time.RFC3339),
			FeedTitle:   article.FeedTitle,
		})
	}
	return result
}

// convertTagsToProto converts domain feed tags to proto format.
func convertTagsToProto(tags []*domain.FeedTag) []*articlesv2.ArticleTagItem {
	result := make([]*articlesv2.ArticleTagItem, 0, len(tags))
	for _, tag := range tags {
		result = append(result, &articlesv2.ArticleTagItem{
			Id:        tag.ID,
			Name:      tag.TagName,
			CreatedAt: tag.CreatedAt.Format(time.RFC3339),
		})
	}
	return result
}

// convertTagCloudItemsToProto converts domain tag cloud items to proto format.
func convertTagCloudItemsToProto(items []*domain.TagCloudItem) []*articlesv2.TagCloudItem {
	result := make([]*articlesv2.TagCloudItem, 0, len(items))
	for _, item := range items {
		result = append(result, &articlesv2.TagCloudItem{
			TagName:      item.TagName,
			ArticleCount: safeconv.Int32(item.ArticleCount),
			PositionX:    float32(item.PositionX),
			PositionY:    float32(item.PositionY),
			PositionZ:    float32(item.PositionZ),
		})
	}
	return result
}

// convertInoreaderSummariesToProto converts inoreader feed summaries to proto format.
func convertInoreaderSummariesToProto(summaries []*domain.InoreaderSummary) []*articlesv2.ArticleSummaryItem {
	result := make([]*articlesv2.ArticleSummaryItem, 0, len(summaries))
	for _, s := range summaries {
		author := ""
		if s.Author != nil {
			author = *s.Author
		}
		result = append(result, &articlesv2.ArticleSummaryItem{
			Title:       s.Title,
			Content:     s.Content,
			Author:      author,
			PublishedAt: s.PublishedAt.Format(time.RFC3339),
			FetchedAt:   s.FetchedAt.Format(time.RFC3339),
			SourceId:    s.InoreaderID,
		})
	}
	return result
}

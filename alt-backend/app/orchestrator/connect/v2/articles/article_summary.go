package articles

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"connectrpc.com/connect"

	"alt/connect/errorhandler"
	articlesv2 "alt/gen/proto/alt/articles/v2"
	"alt/utils/safeconv"
	"alt/utils/security"
)

// FetchArticleSummary fetches article summaries for multiple URLs.
// Replaces POST /v1/articles/summary
// Priority: 1) AI-generated summaries from article_summaries table
//
//  2. Inoreader feed excerpts from inoreader_summaries table
func (h *Handler) FetchArticleSummary(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticleSummaryRequest],
) (*connect.Response[articlesv2.FetchArticleSummaryResponse], error) {
	if _, err := requireUser(ctx); err != nil {
		return nil, err
	}

	feedUrls := req.Msg.FeedUrls

	if len(feedUrls) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("feed_urls cannot be empty"))
	}
	if len(feedUrls) > 50 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("maximum 50 URLs allowed"))
	}

	items := make([]*articlesv2.ArticleSummaryItem, 0, len(feedUrls))

	if h.deps.FetchArticleSummary != nil {
		for _, feedURL := range feedUrls {
			parsedURL, parseErr := url.Parse(feedURL)
			if parseErr != nil {
				h.logger.WarnContext(ctx, "Failed to parse URL for AI summary lookup",
					"url", feedURL,
					"error", parseErr)
				continue
			}

			if ssrfErr := security.NewURLSecurityValidator().ValidateParsedRSSURL(parsedURL); ssrfErr != nil {
				h.logger.WarnContext(ctx, "URL not allowed for AI summary lookup",
					"url", feedURL,
					"error", ssrfErr)
				continue
			}

			aiSummary, aiErr := h.deps.FetchArticleSummary.Execute(ctx, parsedURL)
			if aiErr == nil && aiSummary != nil && aiSummary.Summary != "" {
				items = append(items, &articlesv2.ArticleSummaryItem{
					Title:       "AI Summary",
					Content:     aiSummary.Summary,
					Author:      "",
					PublishedAt: time.Now().Format(time.RFC3339),
					FetchedAt:   time.Now().Format(time.RFC3339),
					SourceId:    "",
				})
			}
		}

		if len(items) > 0 {
			return connect.NewResponse(&articlesv2.FetchArticleSummaryResponse{
				MatchedArticles: items,
				TotalMatched:    safeconv.Int32(len(items)),
				RequestedCount:  safeconv.Int32(len(feedUrls)),
			}), nil
		}
	}

	summaries, err := h.deps.FetchInoreaderSummary.Execute(ctx, feedUrls)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchArticleSummary")
	}

	items = convertInoreaderSummariesToProto(summaries)

	return connect.NewResponse(&articlesv2.FetchArticleSummaryResponse{
		MatchedArticles: items,
		TotalMatched:    safeconv.Int32(len(items)),
		RequestedCount:  safeconv.Int32(len(feedUrls)),
	}), nil
}

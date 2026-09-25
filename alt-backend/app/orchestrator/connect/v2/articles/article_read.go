package articles

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"connectrpc.com/connect"

	"alt/connect/errorhandler"
	articlesv2 "alt/gen/proto/alt/articles/v2"
	"alt/orchestrator/usecase/archive_article_usecase"
	"alt/orchestrator/usecase/get_article_source_url_usecase"
	"alt/utils/security"
)

// FetchArticleContent fetches and extracts compliant article content.
// Replaces GET /v1/articles/fetch/content
func (h *Handler) FetchArticleContent(
	ctx context.Context,
	req *connect.Request[articlesv2.FetchArticleContentRequest],
) (*connect.Response[articlesv2.FetchArticleContentResponse], error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.Url == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("url is required"))
	}

	parsedURL, err := url.Parse(req.Msg.Url)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid URL format: %w", err))
	}

	if err := security.NewURLSecurityValidator().ValidateParsedRSSURL(parsedURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("URL not allowed: %w", err))
	}

	// Call usecase
	forceRefresh := req.Msg.ForceRefresh != nil && *req.Msg.ForceRefresh
	content, articleID, ogImageURL, err := h.deps.Article.FetchCompliantArticleWithRefresh(ctx, parsedURL, *user, forceRefresh)
	if err != nil {
		if upstreamErr := isUpstreamFetchWinner(err); upstreamErr != nil {
			h.logger.WarnContext(ctx, "upstream site did not complete the fetch",
				"url", upstreamErr.URL,
				"cause", upstreamErr.Cause,
				"operation", "FetchArticleContent")
		}
		if mappedErr := mapExternalFetchError(err); mappedErr != nil {
			return nil, mappedErr
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "FetchArticleContent")
	}

	// Content is already sanitized by usecase (ExtractArticleHTML)
	resp := &articlesv2.FetchArticleContentResponse{
		Url:        parsedURL.String(),
		Content:    content,
		ArticleId:  articleID,
		OgImageUrl: ogImageURL,
		// Signed here, with the same signer the feeds handler mints with
		// (see feeds.enrichWithProxyURLs). Without it this RPC's only image
		// field was the publisher's own URL, so every consumer that wanted a
		// thumbnail put a third-party host into an <img src> — an unproxied
		// cross-origin request that skips the rate limiting, SSRF validation,
		// domain allow-list and re-encoding /v1/images/proxy applies.
		//
		// Empty when the image proxy is switched off, which is an explicit
		// operator decision announced at startup by di/image_module.go's
		// image_proxy_disabled log, not an unwired dependency: with no secret
		// there is nothing to sign with. Clients must render no thumbnail
		// rather than fall back to OgImageUrl.
		OgImageProxyUrl: h.signedOgImageURL(ogImageURL),
	}

	return connect.NewResponse(resp), nil
}

// signedOgImageURL returns the HMAC-gated proxy path for an og:image, or "" if
// there is no image or no image proxy configured.
func (h *Handler) signedOgImageURL(ogImageURL string) string {
	if h.deps.ImageProxy == nil || ogImageURL == "" {
		return ""
	}
	return h.deps.ImageProxy.GenerateProxyURL(ogImageURL)
}

// ArchiveArticle archives an article for later reading.
// Replaces POST /v1/articles/archive
func (h *Handler) ArchiveArticle(
	ctx context.Context,
	req *connect.Request[articlesv2.ArchiveArticleRequest],
) (*connect.Response[articlesv2.ArchiveArticleResponse], error) {
	if _, err := requireUser(ctx); err != nil {
		return nil, err
	}

	if req.Msg.FeedUrl == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("feed_url is required"))
	}

	parsedURL, err := url.Parse(req.Msg.FeedUrl)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid URL format: %w", err))
	}

	if err := security.NewURLSecurityValidator().ValidateParsedRSSURL(parsedURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("URL not allowed: %w", err))
	}

	input := archive_article_usecase.ArchiveArticleInput{
		URL:   parsedURL.String(),
		Title: "",
	}
	if req.Msg.Title != nil {
		input.Title = *req.Msg.Title
	}

	if err := h.deps.ArchiveArticle.Execute(ctx, input); err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "ArchiveArticle")
	}

	return connect.NewResponse(&articlesv2.ArchiveArticleResponse{
		Message: "article archived",
	}), nil
}

// GetArticleSourceURL resolves the source URL for an article.
// Fallback for Trail action cards rendered before ingest recorded source_url, or when
// actTargets[].source_url is empty (legacy entry, or producer-side ADR-879
// lookup miss). Read-side query: never appends events.
func (h *Handler) GetArticleSourceURL(
	ctx context.Context,
	req *connect.Request[articlesv2.GetArticleSourceURLRequest],
) (*connect.Response[articlesv2.GetArticleSourceURLResponse], error) {
	user, err := requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if h.deps.GetArticleSourceURL == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("get_article_source_url usecase not wired"))
	}

	source, err := h.deps.GetArticleSourceURL.Execute(ctx, req.Msg.GetArticleId(), user.UserID)
	if err != nil {
		switch {
		case errors.Is(err, get_article_source_url_usecase.ErrInvalidArgument):
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("malformed article_id"))
		case errors.Is(err, get_article_source_url_usecase.ErrNotFound):
			return nil, connect.NewError(connect.CodeNotFound, errors.New("article not found"))
		default:
			h.logger.ErrorContext(ctx, "get_article_source_url failed", "error", err)
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("source url lookup unavailable"))
		}
	}
	return connect.NewResponse(&articlesv2.GetArticleSourceURLResponse{
		SourceUrl: source.URL,
		Title:     source.Title,
	}), nil
}

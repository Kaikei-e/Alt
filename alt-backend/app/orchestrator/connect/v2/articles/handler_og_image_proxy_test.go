package articles

import (
	"log/slog"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	articlesv2 "alt/gen/proto/alt/articles/v2"

	"alt/config"
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/utils/image_proxy"
)

// FetchArticleContent must hand the client a *signed* og:image URL.
//
// The response used to carry only the raw publisher URL. Every consumer of
// that field therefore put a third-party host straight into an <img src>: an
// unproxied cross-origin request, outside the HMAC-gated /v1/images/proxy
// path, with none of the rate limiting, SSRF validation, domain allow-listing
// or re-encoding the proxy exists to apply. The frontend's swipe surface read
// it through the article prefetcher and rendered it verbatim.
//
// The field to fix it (og_image_proxy_url, field 5) already existed on the
// message and was already read by the frontend client — nothing was ever
// written into it. The signer is the same one the feeds handler mints with,
// and it is already in this handler's Deps for BatchPrefetchImages, so this
// is a wiring gap rather than a missing capability.
func TestFetchArticleContent_SignsTheOgImageURL(t *testing.T) {
	// Skips the SSRF resolver's DNS lookup: the host is allow-listed, so the
	// test does not depend on name resolution being available.
	t.Setenv("FEED_ALLOWED_HOSTS", "example.com")

	const rawOgImage = "https://cdn.example.com/hero.jpg"
	signer := image_proxy.NewSigner("test-secret")

	handler := NewHandler(ArticleHandlerDeps{
		Article: &mockArticleUsecase{
			content:    "<p>Body.</p>",
			articleID:  "article-1",
			ogImageURL: rawOgImage,
		},
		ImageProxy: image_proxy_usecase.NewImageProxyUsecase(
			nil, nil, nil, signer, nil, nil, 0, 0, 0),
	}, &config.Config{}, slog.Default())

	resp, err := handler.FetchArticleContent(createAuthContext(),
		connect.NewRequest(&articlesv2.FetchArticleContentRequest{
			Url: "https://example.com/article",
		}))

	require.NoError(t, err)
	assert.Equal(t, signer.GenerateProxyURL(rawOgImage), resp.Msg.OgImageProxyUrl)
	assert.True(t, strings.HasPrefix(resp.Msg.OgImageProxyUrl, "/v1/images/proxy/"),
		"the proxy URL must be the HMAC-gated path, got %q", resp.Msg.OgImageProxyUrl)
	// The raw URL still travels — a caller that wants the origin's own URL for
	// something other than an <img src> is not the problem being fixed — but it
	// is no longer the only thing on offer.
	assert.Equal(t, rawOgImage, resp.Msg.OgImageUrl)
}

// An article with no og:image gets no signed URL, rather than a signature over
// the empty string.
func TestFetchArticleContent_NoOgImage_LeavesProxyURLEmpty(t *testing.T) {
	t.Setenv("FEED_ALLOWED_HOSTS", "example.com")

	handler := NewHandler(ArticleHandlerDeps{
		Article: &mockArticleUsecase{content: "<p>Body.</p>", articleID: "article-1"},
		ImageProxy: image_proxy_usecase.NewImageProxyUsecase(
			nil, nil, nil, image_proxy.NewSigner("test-secret"), nil, nil, 0, 0, 0),
	}, &config.Config{}, slog.Default())

	resp, err := handler.FetchArticleContent(createAuthContext(),
		connect.NewRequest(&articlesv2.FetchArticleContentRequest{
			Url: "https://example.com/article",
		}))

	require.NoError(t, err)
	assert.Empty(t, resp.Msg.OgImageProxyUrl)
}

// IMAGE_PROXY_ENABLED=false is an explicit operator decision, announced by
// di/image_module.go's image_proxy_disabled startup log, and it leaves
// ImageProxyUsecase nil for the whole process. That is the one case where an
// empty proxy URL is correct rather than a wiring bug (CLAUDE.md rules 8/9):
// there is no secret to sign with, so the field stays empty and the client
// renders no thumbnail — it must never fall back to the raw URL.
func TestFetchArticleContent_ImageProxyDisabled_LeavesProxyURLEmpty(t *testing.T) {
	t.Setenv("FEED_ALLOWED_HOSTS", "example.com")

	handler := NewHandler(ArticleHandlerDeps{
		Article: &mockArticleUsecase{
			content:    "<p>Body.</p>",
			articleID:  "article-1",
			ogImageURL: "https://cdn.example.com/hero.jpg",
		},
	}, &config.Config{}, slog.Default())

	resp, err := handler.FetchArticleContent(createAuthContext(),
		connect.NewRequest(&articlesv2.FetchArticleContentRequest{
			Url: "https://example.com/article",
		}))

	require.NoError(t, err)
	assert.Empty(t, resp.Msg.OgImageProxyUrl)
}

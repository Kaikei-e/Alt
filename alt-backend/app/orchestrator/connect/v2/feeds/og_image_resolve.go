package feeds

import (
	"context"
	"errors"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	feedsv2 "alt/gen/proto/alt/feeds/v2"

	"connectrpc.com/connect"
)

// ResolveOgImages obtains og:image URLs for feeds that arrived without one, at
// the moment a reader brings those cards into view.
//
// Feeds that could not be resolved are omitted from `images` rather than
// returned with an empty URL: the client distinguishes "no image" from "not
// asked yet" by absence, and collapsing the two would make it ask again on
// every scroll.
//
// `unresolved` then says which kind of absence it is, and how long the client
// should wait before putting the question again. The four outcomes and their
// encoding are documented on ResolveOgImagesResponse in
// proto/alt/feeds/v2/feeds.proto; this handler is a straight transcription of
// the usecase's two maps, which is where they are decided.
func (h *Handler) ResolveOgImages(
	ctx context.Context,
	req *connect.Request[feedsv2.ResolveOgImagesRequest],
) (*connect.Response[feedsv2.ResolveOgImagesResponse], error) {
	if _, err := middleware.GetUserContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	feedIDs := req.Msg.GetFeedIds()
	if len(feedIDs) == 0 {
		return connect.NewResponse(&feedsv2.ResolveOgImagesResponse{}), nil
	}

	if h.deps.ResolveOgImages == nil {
		// Rule 8: an unwired resolver must not be indistinguishable from "no
		// feed has an image". The only configuration in which resolution is
		// legitimately absent is IMAGE_PROXY_ENABLED=false, and that is stated
		// at startup — so from inside business code, nil here is a wiring bug
		// and it says so instead of answering empty.
		return nil, connect.NewError(connect.CodeUnimplemented,
			errors.New("og image resolution is not available: the image proxy is disabled"))
	}

	resolved, unresolved, err := h.deps.ResolveOgImages.Execute(ctx, feedIDs)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "ResolveOgImages")
	}

	images := make([]*feedsv2.ResolveOgImage, 0, len(resolved))
	for feedID, proxyURL := range resolved {
		images = append(images, &feedsv2.ResolveOgImage{
			FeedId:          feedID,
			OgImageProxyUrl: proxyURL,
		})
	}

	// A zero retry_after_seconds is a meaningful answer here — "settled inside
	// this retention window" — so these rows are emitted whatever the number
	// is. Under protojson the zero travels as an absent key, which is what the
	// contract comment describes and what the client reads as permanent.
	unresolvedOut := make([]*feedsv2.UnresolvedOgImage, 0, len(unresolved))
	for feedID, seconds := range unresolved {
		unresolvedOut = append(unresolvedOut, &feedsv2.UnresolvedOgImage{
			FeedId:            feedID,
			RetryAfterSeconds: seconds,
		})
	}

	return connect.NewResponse(&feedsv2.ResolveOgImagesResponse{
		Images:     images,
		Unresolved: unresolvedOut,
	}), nil
}

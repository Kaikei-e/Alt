import * as v from "valibot";
import { safeUrlSchema } from "./urlValidation";

export type FeedUrlInput = {
  feed_url: string;
};

export const feedUrlSchema = v.object({
  feed_url: v.pipe(
    v.string("Invalid or unsafe URL"),
    v.transform((url) => url.trim()),
    v.transform((url) => url.replace(/^@/, "")), // Remove @ prefix if present
    safeUrlSchema,
    v.check(
      (url) => isLikelyRssFeed(url),
      "URL does not appear to be a valid RSS or Atom feed",
    ),
  ),
});

function isLikelyRssFeed(url: string): boolean {
  // Common RSS/Atom feed patterns
  const feedPatterns = [
    /\/rss\/?(\?|#|$)/i,
    /\/rss\/.+/i, // /rss/ followed by path
    /\/rss\d+\/?(\?|#|$)/i, // /rss2, /rss20, etc.
    /\/rss-feed\/?/i, // /rss-feed/ or /rss-feed
    /\/rss-feeds\/?/i, // /rss-feeds/ or /rss-feeds
    /\/rssfeed\/?/i, // /rssfeed/ or /rssfeed
    /\/rss_feeds\/?/i, // /rss_feeds/ or /rss_feeds
    /\/feed\/?(\?|#|$)/i,
    /\/feed\/.+/i, // /feed/ followed by path
    /\/atom\/?(\?|#|$)/i,
    /\/atom\/.+/i, // /atom/ followed by path
    /\.xml(\?|#|$)/i,
    /\.rss(\?|#|$)/i,
    /\.rdf(\?|#|$)/i, // RDF files
    /\/rss\.xml(\?|#|$)/i,
    /\/feed\.xml(\?|#|$)/i,
    /\/atom\.xml(\?|#|$)/i,
    /feeds\.feedburner\.com/i,
    /\/feeds\//i,
    /\/index\.rdf(\?|#|$)/i, // index.rdf files
  ];

  return feedPatterns.some((pattern) => pattern.test(url));
}



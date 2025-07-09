import * as v from "valibot";
import { safeUrlSchema } from "./urlValidation";

export type FeedUrlInput = {
  feed_url: string;
};

export const feedUrlSchema = v.object({
  feed_url: v.pipe(
    safeUrlSchema,
    v.check((url) => {
      // Additional validation for RSS/Atom feeds
      return isLikelyRssFeed(url);
    }, "URL does not appear to be a valid RSS or Atom feed"),
  ),
});

function isLikelyRssFeed(url: string): boolean {
  // Common RSS/Atom feed patterns
  const feedPatterns = [
    /\/rss\/?(\?|#|$)/i,
    /\/feed\/?(\?|#|$)/i,
    /\/atom\/?(\?|#|$)/i,
    /\.xml(\?|#|$)/i,
    /\.rss(\?|#|$)/i,
    /\/rss\.xml(\?|#|$)/i,
    /\/feed\.xml(\?|#|$)/i,
    /\/atom\.xml(\?|#|$)/i,
    /feeds\.feedburner\.com/i,
    /\/feeds\//i,
  ];

  return feedPatterns.some(pattern => pattern.test(url));
}
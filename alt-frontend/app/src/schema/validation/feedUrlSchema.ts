import * as v from "valibot";
import { safeUrlSchema } from "./urlValidation";

export type FeedUrlInput = {
  feed_url: string;
};

export const feedUrlSchema = v.object({
  feed_url: v.pipe(
    v.string("Invalid or unsafe URL"),
    v.transform((url) => url.replace(/^@/, '')), // Remove @ prefix if present
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
    /\/rss\d+\/?(\?|#|$)/i,  // /rss2, /rss20, etc.
    /\/feed\/?(\?|#|$)/i,
    /\/atom\/?(\?|#|$)/i,
    /\.xml(\?|#|$)/i,
    /\.rss(\?|#|$)/i,
    /\.rdf(\?|#|$)/i,        // RDF files
    /\/rss\.xml(\?|#|$)/i,
    /\/feed\.xml(\?|#|$)/i,
    /\/atom\.xml(\?|#|$)/i,
    /feeds\.feedburner\.com/i,
    /\/feeds\//i,
    /\/index\.rdf(\?|#|$)/i, // index.rdf files
  ];

  return feedPatterns.some((pattern) => pattern.test(url));
}

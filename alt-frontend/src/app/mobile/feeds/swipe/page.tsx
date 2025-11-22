import { serverFetch } from "@/lib/api/utils/serverFetch";
import { fetchArticleContentServer } from "@/lib/api/utils/serverArticleFetch";
import type { CursorResponse } from "@/schema/common";
import type { Feed, BackendFeedItem } from "@/schema/feed";
import { sanitizeFeed } from "@/schema/feed";
import type { SafeHtmlString } from "@/lib/server/sanitize-html";

import SwipeFeedScreen from "@/components/mobile/feeds/swipe/SwipeFeedScreen";

/**
 * Fetches the first feed from the cursor API
 */
async function fetchFirstFeed(): Promise<Feed | null> {
  try {
    const response = await serverFetch<CursorResponse<BackendFeedItem>>(
      "/v1/feeds/fetch/cursor?limit=1",
    );

    if (response.data && response.data.length > 0) {
      return sanitizeFeed(response.data[0]);
    }

    return null;
  } catch (error) {
    console.error("[SwipeFeedsPage] Error fetching first feed:", error);
    return null;
  }
}

/**
 * Fetches article content for the first feed
 */
async function fetchFirstArticleContent(
  feedUrl: string,
): Promise<SafeHtmlString | null> {
  try {
    const response = await fetchArticleContentServer(feedUrl);
    return response.content;
  } catch (error) {
    // Log error but don't throw - we can still show the feed without article content
    console.error(
      "[SwipeFeedsPage] Error fetching article content:",
      error,
    );
    return null;
  }
}

export default async function SwipeFeedsPage() {
  // Fetch first feed
  const firstFeed = await fetchFirstFeed();

  // If we have a feed, fetch article content in parallel (non-blocking)
  const articleContentPromise = firstFeed?.link
    ? fetchFirstArticleContent(firstFeed.link)
    : Promise.resolve(null);

  // Wait for article content (but don't block if it fails)
  const articleContent = await articleContentPromise;

  return (
    <SwipeFeedScreen
      initialFeed={firstFeed}
      initialArticleContent={articleContent}
    />
  );
}

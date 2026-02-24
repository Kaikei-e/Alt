import SwipeFeedScreen from "@/components/mobile/feeds/swipe/SwipeFeedScreen";
import { fetchArticleContentServer } from "@/lib/api/utils/serverArticleFetch";
import { serverFetch } from "@/lib/api/utils/serverFetch";
import type { SafeHtmlString } from "@/lib/server/sanitize-html";
import type { CursorResponse } from "@/schema/common";
import type { BackendFeedItem, RenderFeed } from "@/schema/feed";
import { sanitizeFeed, toRenderFeed } from "@/schema/feed";

const INITIAL_FEEDS_LIMIT = 3; // Reduced from 5 to 3 for LCP optimization

/**
 * Fetches the initial feeds from the cursor API
 */
async function fetchInitialFeeds(): Promise<{
  feeds: RenderFeed[];
  nextCursor?: string;
}> {
  try {
    const response = await serverFetch<CursorResponse<BackendFeedItem>>(
      `/v1/feeds/fetch/cursor?limit=${INITIAL_FEEDS_LIMIT}`,
    );

    if (response.data && response.data.length > 0) {
      return {
        feeds: response.data.map((item) => {
          const sanitized = sanitizeFeed(item);
          return toRenderFeed(sanitized, item.tags);
        }),
        nextCursor: response.next_cursor || undefined,
      };
    }

    return { feeds: [] };
  } catch (error) {
    console.error("[SwipeFeedsPage] Error fetching initial feeds:", error);
    return { feeds: [] };
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
    console.error("[SwipeFeedsPage] Error fetching article content:", error);
    return null;
  }
}

export default async function SwipeFeedsPage() {
  // Fetch initial feeds
  const { feeds, nextCursor } = await fetchInitialFeeds();
  const firstFeed = feeds[0] ?? null;

  // If we have a feed, fetch article content in parallel (non-blocking)
  const articleContentPromise = firstFeed?.link
    ? fetchFirstArticleContent(firstFeed.link)
    : Promise.resolve(null);

  // Wait for article content (but don't block if it fails)
  const articleContent = await articleContentPromise;

  return (
    <SwipeFeedScreen
      initialFeeds={feeds}
      initialNextCursor={nextCursor}
      initialArticleContent={articleContent}
    />
  );
}

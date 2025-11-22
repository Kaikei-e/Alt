import { Suspense } from "react";
import dynamic from "next/dynamic";
import { serverFetch } from "@/lib/api/utils/serverFetch";
import { fetchArticleContentServer } from "@/lib/api/utils/serverArticleFetch";
import type { CursorResponse } from "@/schema/common";
import type { Feed, BackendFeedItem } from "@/schema/feed";
import { sanitizeFeed } from "@/schema/feed";
import type { SafeHtmlString } from "@/lib/server/sanitize-html";

// Load SwipeFeedScreen dynamically to reduce initial bundle
// Note: ssr: false is not allowed in Server Components, so we remove it
// The component itself is already a client component, so it will be client-side only
const SwipeFeedScreen = dynamic(
  () => import("@/components/mobile/feeds/swipe/SwipeFeedScreen"),
  {
    loading: () => (
      <div
        style={{
          minHeight: "100dvh",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <div>Loading...</div>
      </div>
    ),
  }
);

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

/**
 * Server component that fetches initial data
 */
async function SwipeFeedsPageContent() {
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

export default function SwipeFeedsPage() {
  return (
    <Suspense fallback={<div>Loading...</div>}>
      <SwipeFeedsPageContent />
    </Suspense>
  );
}

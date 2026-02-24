import { Box } from "@chakra-ui/react";
import { Suspense } from "react";
import { serverFetch } from "@/lib/api/utils/serverFetch";
import type { CursorResponse } from "@/schema/common";
import type { BackendFeedItem, RenderFeed } from "@/schema/feed";
import { sanitizeFeed, toRenderFeed } from "@/schema/feed";
import { FeedsClient } from "./_components/FeedsClient";
import { MobileFeedsHero } from "./_components/MobileFeedsHero";

const INITIAL_FEEDS_LIMIT = 20; // Fetch 20, but render only 3 initially

/**
 * Fetches initial feeds from the cursor API and converts them to RenderFeed
 */
async function fetchInitialFeeds(): Promise<RenderFeed[]> {
  try {
    const response = await serverFetch<CursorResponse<BackendFeedItem>>(
      `/v1/feeds/fetch/cursor?limit=${INITIAL_FEEDS_LIMIT}`,
    );

    if (response.data && response.data.length > 0) {
      return response.data.map((item) => {
        const sanitized = sanitizeFeed(item);
        return toRenderFeed(sanitized, item.tags);
      });
    }

    return [];
  } catch (error) {
    console.error("[FeedsPage] Error fetching initial feeds:", error);
    return [];
  }
}

/**
 * Mobile Feeds Page (Server Component)
 *
 * This page is split into Server and Client components to optimize LCP:
 * - Hero section (MobileFeedsHero) is rendered on the server for immediate display
 * - Initial feeds are fetched and converted to RenderFeed on the server
 * - Feed list and interactions (FeedsClient) are handled on the client
 *
 * The Hero section with the LCP-optimized tip is always rendered first,
 * ensuring consistent LCP timing.
 */
export default async function FeedsPage() {
  // Fetch and convert feeds on the server
  const initialFeeds = await fetchInitialFeeds();

  return (
    <Box minH="100dvh" display="flex" flexDirection="column">
      <MobileFeedsHero />
      <Box flex="1" position="relative" mt={2}>
        <Suspense fallback={null}>
          <FeedsClient initialFeeds={initialFeeds} />
        </Suspense>
      </Box>
    </Box>
  );
}

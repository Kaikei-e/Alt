"use client";

import { Flex, Text, Box } from "@chakra-ui/react";
import { feedsApi } from "@/lib/api";
import { Feed } from "@/schema/feed";
import SkeletonFeedCard from "@/components/mobile/SkeletonFeedCard";
import VirtualFeedList from "@/components/mobile/VirtualFeedList";
import { useRef, useState, useCallback, useMemo, startTransition } from "react";
import { useEffect } from "react";
import { useInfiniteScroll } from "@/lib/utils/infiniteScroll";
import { useCursorPagination } from "@/hooks/useCursorPagination";
import ErrorState from "./_components/ErrorState";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";

const PAGE_SIZE = 20;

// URL正規化関数（TODO.mdの指示に基づく）
const canonicalize = (url: string) => {
  try {
    const u = new URL(url);
    u.hash = "";
    ["utm_source","utm_medium","utm_campaign","utm_term","utm_content"].forEach(k => u.searchParams.delete(k));
    if (u.pathname !== "/" && u.pathname.endsWith("/")) u.pathname = u.pathname.slice(0, -1);
    return u.toString();
  } catch { return url }
};

export default function FeedsPage() {
  const [readFeeds, setReadFeeds] = useState<Set<string>>(new Set());
  const [liveRegionMessage, setLiveRegionMessage] = useState<string>("");
  const [isRetrying, setIsRetrying] = useState(false);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  // Ensure we start at the top of the list on first render (some mobile browsers
  // restore scroll position across navigations, which can leave us mid-way in
  // a virtualized list with no items rendered yet).
  useEffect(() => {
    const el = scrollContainerRef.current;
    if (el) {
      el.scrollTop = 0;
    }
  }, []);

  // Use cursor-based pagination hook
  const {
    data: feeds,
    hasMore,
    isLoading,
    error,
    isInitialLoading,
    loadMore,
    refresh,
  } = useCursorPagination<Feed>(feedsApi.getFeedsWithCursor, {
    limit: PAGE_SIZE,
    autoLoad: true,
  });

  // Memoize visible feeds to prevent unnecessary recalculations
  const visibleFeeds = useMemo(
    () => feeds?.filter((feed) => !readFeeds.has(feed.link)) || [],
    [feeds, readFeeds],
  );

  // Handle marking feed as read with optimistic update + API call (TODO.mdの指示に基づく)
  const handleMarkAsRead = useCallback(async (rawLink: string) => {
    const link = canonicalize(rawLink);

    // 楽観更新（即時にUIから消す）
    startTransition(() => {
      setReadFeeds(prev => new Set(prev).add(link));
    });
    setLiveRegionMessage("Feed marked as read");
    setTimeout(() => setLiveRegionMessage(""), 1000);

    // サーバ更新（失敗時はロールバック）
    try {
      await feedsApi.updateFeedReadStatus(link);
    } catch (e) {
      startTransition(() => {
        setReadFeeds(prev => {
          const next = new Set(prev);
          next.delete(link);
          return next;
        });
      });
      // エラートーストの表示（TODO: 必要に応じてトースト表示を追加）
      console.error("Failed to mark feed as read:", e);
    }
  }, []);

  // Retry functionality with exponential backoff
  const retryFetch = useCallback(async () => {
    setIsRetrying(true);

    try {
      await refresh();
    } catch (err) {
      console.error("Retry failed:", err);
      throw err; // Re-throw to let ErrorState handle retry logic
    } finally {
      setIsRetrying(false);
    }
  }, [refresh]);

  // Use infinite scroll hook with proper callback
  const handleLoadMore = useCallback(() => {
    if (hasMore && !isLoading) {
      loadMore();
    }
  }, [hasMore, isLoading, loadMore]);

  useInfiniteScroll(handleLoadMore, sentinelRef, feeds?.length || 0, {
    throttleDelay: 200, // Increased throttle for better stability
    rootMargin: "100px 0px", // Trigger loading a bit earlier
    threshold: 0.1,
  });

  // Show skeleton loading state for immediate visual feedback
  if (isInitialLoading) {
    return (
      <Box minH="100vh" position="relative">
        <Box
          p={5}
          maxW="container.sm"
          mx="auto"
          height="100vh"
          data-testid="feeds-skeleton-container"
        >
          <Flex direction="column" gap={4}>
            {/* Render 5 skeleton cards for immediate visual feedback */}
            {Array.from({ length: 5 }).map((_, index) => (
              <SkeletonFeedCard key={`skeleton-${index}`} />
            ))}
          </Flex>
        </Box>

        <FloatingMenu />
      </Box>
    );
  }

  // Show error state
  if (error) {
    return (
      <ErrorState error={error} onRetry={retryFetch} isLoading={isRetrying} />
    );
  }

  return (
    <Box minH="100vh" position="relative">
      <Box
        aria-live="polite"
        aria-atomic="true"
        position="absolute"
        left="-10000px"
        width="1px"
        height="1px"
        overflow="hidden"
      >
        {liveRegionMessage}
      </Box>

      <Box
        ref={scrollContainerRef}
        p={5}
        maxW="container.sm"
        mx="auto"
        overflowY="auto"
        overflowX="hidden"
        height="100vh"
        data-testid="feeds-scroll-container"
        bg="var(--app-bg)"
      >
        {visibleFeeds.length > 0 ? (
          <>
            {/* Feed list rendering */}
            <VirtualFeedList
              feeds={visibleFeeds}
              readFeeds={readFeeds}
              onMarkAsRead={handleMarkAsRead}
            />

            {/* No more feeds indicator */}
            {!hasMore && visibleFeeds.length > 0 && (
              <Text
                textAlign="center"
                color="var(--alt-text-secondary)"
                fontSize="sm"
                mt={8}
                mb={4}
              >
                No more feeds to load
              </Text>
            )}
          </>
        ) : (
          /* Empty state */
          <Flex justify="center" align="center" py={20}>
            <Text color="var(--alt-text-secondary)" fontSize="lg">
              No feeds available
            </Text>
          </Flex>
        )}

        {/* Infinite scroll sentinel - always rendered when feeds are present and there's more to load */}
        {visibleFeeds.length > 0 && hasMore && (
          <div
            ref={sentinelRef}
            style={{
              height: "50px",
              width: "100%",
              backgroundColor: "transparent",
              margin: "10px 0",
              position: "relative",
              zIndex: 1,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              flexShrink: 0,
            }}
            data-testid="infinite-scroll-sentinel"
          >
            {/* Loading more indicator inside sentinel */}
            {isLoading && (
              <Text color="var(--alt-text-secondary)" fontSize="sm">
                Loading more...
              </Text>
            )}
          </div>
        )}
      </Box>

      <FloatingMenu />
    </Box>
  );
}

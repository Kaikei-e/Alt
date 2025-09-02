"use client";

import { Flex, Text, Box } from "@chakra-ui/react";
import { feedsApi } from "@/lib/api";
import { Feed } from "@/schema/feed";
import FeedCard from "@/components/mobile/FeedCard";
import SkeletonFeedCard from "@/components/mobile/SkeletonFeedCard";
import { useRef, useState, useCallback, useMemo } from "react";
import { useInfiniteScroll } from "@/lib/utils/infiniteScroll";
import { useCursorPagination } from "@/hooks/useCursorPagination";
import ErrorState from "../_components/ErrorState";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";

const PAGE_SIZE = 20;

export default function FavoriteFeedsPage() {
  const [readFeeds, setReadFeeds] = useState<Set<string>>(new Set());
  const [liveRegionMessage, setLiveRegionMessage] = useState<string>("");
  const [isRetrying, setIsRetrying] = useState(false);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  const {
    data: feeds,
    hasMore,
    isLoading,
    error,
    isInitialLoading,
    loadMore,
    refresh,
  } = useCursorPagination<Feed>(feedsApi.getFavoriteFeedsWithCursor, {
    limit: PAGE_SIZE,
    autoLoad: true,
  });

  const visibleFeeds = useMemo(
    () => feeds?.filter((feed) => !readFeeds.has(feed.link)) || [],
    [feeds, readFeeds],
  );

  const handleMarkAsRead = useCallback((feedLink: string) => {
    setReadFeeds((prev) => {
      const newSet = new Set(prev);
      newSet.add(feedLink);
      setLiveRegionMessage(`Feed marked as read`);
      setTimeout(() => setLiveRegionMessage(""), 1000);
      return newSet;
    });
  }, []);

  const retryFetch = useCallback(async () => {
    setIsRetrying(true);

    try {
      await refresh();
    } catch (err) {
      console.error("Retry failed:", err);
      throw err;
    } finally {
      setIsRetrying(false);
    }
  }, [refresh]);

  const handleLoadMore = useCallback(() => {
    if (hasMore && !isLoading) {
      loadMore();
    }
  }, [hasMore, isLoading, loadMore]);

  useInfiniteScroll(handleLoadMore, sentinelRef, feeds?.length || 0, {
    throttleDelay: 200,
    rootMargin: "100px 0px",
    threshold: 0.1,
  });

  if (isInitialLoading) {
    return (
      <Box minH="100vh" position="relative">
        <Box
          p={5}
          maxW="container.sm"
          mx="auto"
          height="100vh"
          data-testid="favorites-skeleton-container"
        >
          <Flex direction="column" gap={4}>
            {Array.from({ length: 5 }).map((_, index) => (
              <SkeletonFeedCard key={`skeleton-${index}`} />
            ))}
          </Flex>
        </Box>

        <FloatingMenu />
      </Box>
    );
  }

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
        data-testid="favorites-scroll-container"
        bg="var(--app-bg)"
      >
        {visibleFeeds.length > 0 ? (
          <>
            <Flex direction="column" gap={4}>
              {visibleFeeds.map((feed: Feed) => (
                <FeedCard
                  key={feed.link}
                  feed={feed}
                  isReadStatus={readFeeds.has(feed.link)}
                  setIsReadStatus={() => handleMarkAsRead(feed.link)}
                />
              ))}
            </Flex>

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
          <Flex justify="center" align="center" py={20}>
            <Text color="var(--alt-text-secondary)" fontSize="lg">
              No feeds available
            </Text>
          </Flex>
        )}

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

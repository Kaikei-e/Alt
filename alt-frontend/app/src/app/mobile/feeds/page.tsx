"use client";

import { Flex, Text, Box } from "@chakra-ui/react";
import { feedsApi } from "@/lib/api";
import { Feed } from "@/schema/feed";
import FeedCard from "@/components/mobile/FeedCard";
import { useEffect, useRef, useState, useCallback, useMemo } from "react";
import { useInfiniteScroll } from "@/lib/utils/infiniteScroll";
import { useCursorPagination } from "@/hooks/useCursorPagination";
import ErrorState from "./_components/ErrorState";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";

const PAGE_SIZE = 20;
const VIRTUAL_SCROLL_THRESHOLD = 50; // Start virtual scrolling after 50 items

export default function FeedsPage() {
  const [readFeeds, setReadFeeds] = useState<Set<string>>(new Set());
  const [liveRegionMessage, setLiveRegionMessage] = useState<string>("");
  const [isRetrying, setIsRetrying] = useState(false);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const [visibleRange, setVisibleRange] = useState({ start: 0, end: 25 });
  const sentinelRef = useRef<HTMLDivElement>(null);

  // Use cursor-based pagination hook
  const {
    data: feeds,
    hasMore,
    isLoading,
    error,
    isInitialLoading,
    loadInitial,
    loadMore,
    refresh,
  } = useCursorPagination(feedsApi.getFeedsWithCursor, { limit: PAGE_SIZE });

  // Memoize visible feeds to prevent unnecessary recalculations
  const visibleFeeds = useMemo(
    () => feeds.filter((feed) => !readFeeds.has(feed.link)),
    [feeds, readFeeds],
  );

  // Virtual scrolling for large lists - Re-enabled for performance
  const shouldUseVirtualScrolling = visibleFeeds.length > VIRTUAL_SCROLL_THRESHOLD;
  const virtualizedFeeds = useMemo(() => {
    if (!shouldUseVirtualScrolling) {
      return visibleFeeds;
    }
    return visibleFeeds.slice(visibleRange.start, visibleRange.end);
  }, [visibleFeeds, shouldUseVirtualScrolling, visibleRange]);

  // Handle marking feed as read
  const handleMarkAsRead = useCallback((feedLink: string) => {
    setReadFeeds((prev) => {
      const newSet = new Set(prev);
      newSet.add(feedLink);

      // Update live region for screen readers
      setLiveRegionMessage(`Feed marked as read`);
      setTimeout(() => setLiveRegionMessage(""), 1000);

      return newSet;
    });
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

  // Initialize feeds
  useEffect(() => {
    loadInitial();
  }, [loadInitial]);

  // Use infinite scroll hook with proper callback
  const handleLoadMore = useCallback(() => {
    if (hasMore && !isLoading) {
      loadMore();
    }
  }, [hasMore, isLoading, loadMore]);

  useInfiniteScroll(handleLoadMore, sentinelRef, feeds.length, {
    throttleDelay: 50, // Reduce throttle for better test performance
    rootMargin: "100px 0px", // Reduced margin for faster triggering
    threshold: 0.1,
  });

  // Virtual scroll effect
  useEffect(() => {
    if (!shouldUseVirtualScrolling) return;

    const container = scrollContainerRef.current;
    if (!container) return;

    const updateVisibleRange = () => {
      const scrollTop = container.scrollTop;
      const containerHeight = container.clientHeight;
      const itemHeight = 200; // Approximate height per feed card

      const start = Math.max(0, Math.floor(scrollTop / itemHeight) - 2);
      // Cap the visible range to maintain performance - show at most 25 items at once
      const visibleCount = Math.min(25, Math.ceil(containerHeight / itemHeight) + 5);
      const end = Math.min(visibleFeeds.length, start + visibleCount);

      setVisibleRange({ start, end });
    };

    container.addEventListener('scroll', updateVisibleRange);
    updateVisibleRange(); // Initial calculation

    return () => container.removeEventListener('scroll', updateVisibleRange);
  }, [shouldUseVirtualScrolling, visibleFeeds.length]);



  // Show loading state with proper spinner
  if (isInitialLoading) {
    return (
      <Flex
        justify="center"
        align="center"
        minH="100vh"
        direction="column"
        gap={4}
      >
        <Box data-testid="loading-spinner">
          <Box className="glass" p={8} borderRadius="20px" backdropFilter="blur(10px)">
            <Flex direction="column" align="center" gap={4}>
              <svg
                width="48"
                height="48"
                viewBox="0 0 100 100"
                style={{
                  animation: "spin 1s linear infinite",
                }}
              >
                <circle
                  cx="50"
                  cy="50"
                  r="40"
                  stroke="#ff006e"
                  strokeWidth="8"
                  fill="none"
                  strokeLinecap="round"
                  strokeDasharray="60 40"
                />
              </svg>
              <style jsx>{`
                @keyframes spin {
                  0% { transform: rotate(0deg); }
                  100% { transform: rotate(360deg); }
                }
              `}</style>
              <Text color="rgba(255, 255, 255, 0.8)" fontSize="sm">
                Loading feeds...
              </Text>
            </Flex>
          </Box>
        </Box>
      </Flex>
    );
  }

  // Show error state
  if (error) {
    return (
      <ErrorState
        error={error}
        onRetry={retryFetch}
        isLoading={isRetrying}
      />
    );
  }

  return (
    <Box minH="100vh" position="relative">
      {/* ARIA live region for announcements */}
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
        height="100vh"
        data-testid="feeds-scroll-container"
      >
        {/* Header */}
        <Text
          fontSize="2xl"
          fontWeight="bold"
          color="#ff006e"
          mb={6}
          textAlign="center"
        >
          Latest Feeds
        </Text>

        {visibleFeeds.length > 0 ? (
          <>
            {/* Virtual scrolling spacer for items before visible range */}
            {shouldUseVirtualScrolling && visibleRange.start > 0 && (
              <Box height={`${visibleRange.start * 200}px`} width="100%" />
            )}

            {/* Feed Cards */}
            <Flex direction="column" gap={4}>
              {virtualizedFeeds.map((feed: Feed) => (
                <FeedCard
                  key={feed.link}
                  feed={feed}
                  isReadStatus={readFeeds.has(feed.link)}
                  setIsReadStatus={() => handleMarkAsRead(feed.link)}
                />
              ))}
            </Flex>

            {/* Virtual scrolling spacer for items after visible range */}
            {shouldUseVirtualScrolling && visibleRange.end < visibleFeeds.length && (
              <Box height={`${(visibleFeeds.length - visibleRange.end) * 200}px`} width="100%" />
            )}

            {/* No more feeds indicator */}
            {!hasMore && visibleFeeds.length > 0 && (
              <Text
                textAlign="center"
                color="whiteAlpha.600"
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
            <Text color="rgba(255, 255, 255, 0.6)" fontSize="lg">
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
              <Text color="rgba(255, 255, 255, 0.8)" fontSize="sm">
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

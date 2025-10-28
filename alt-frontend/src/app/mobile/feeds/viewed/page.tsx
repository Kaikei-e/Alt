"use client";

import { Flex, Text, Box } from "@chakra-ui/react";
import { Feed } from "@/schema/feed";
import ReadFeedCard from "@/components/mobile/ReadFeedCard";
import SkeletonFeedCard from "@/components/mobile/SkeletonFeedCard";
import { useRef, useState, useCallback, Component, ErrorInfo, ReactNode } from "react";
import { useInfiniteScroll } from "@/lib/utils/infiniteScroll";
import { useReadFeeds } from "@/hooks/useReadFeeds";
import ErrorState from "../_components/ErrorState";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";

// Error boundary to catch React errors
class ViewedFeedsErrorBoundary extends Component<
  { children: ReactNode },
  { hasError: boolean; error: Error | null }
> {
  constructor(props: { children: ReactNode }) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error) {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error("[ViewedFeeds] Error caught by boundary:", error, errorInfo);
  }

  render() {
    if (this.state.hasError) {
      return (
        <Box p={5}>
          <Text color="red.500" fontSize="lg" fontWeight="bold">
            エラーが発生しました
          </Text>
          <Text color="red.400" fontSize="sm" mt={2}>
            {this.state.error?.message || "Unknown error"}
          </Text>
          <Text color="gray.500" fontSize="xs" mt={4}>
            {this.state.error?.stack}
          </Text>
        </Box>
      );
    }

    return this.props.children;
  }
}

function ReadFeedsPageContent() {
  const [liveRegionMessage, setLiveRegionMessage] = useState<string>("");
  const [isRetrying, setIsRetrying] = useState(false);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  // Use the useReadFeeds hook for data management
  const { feeds, isLoading, error, hasMore, loadMore, refresh } =
    useReadFeeds(20);

  // Check if this is initial loading (no feeds yet and loading)
  const isInitialLoading = isLoading && feeds.length === 0;

  // Retry functionality with exponential backoff
  const retryFetch = useCallback(async () => {
    setIsRetrying(true);

    try {
      await refresh();
      setLiveRegionMessage("Read feeds refreshed successfully");
      setTimeout(() => setLiveRegionMessage(""), 1000);
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

  useInfiniteScroll(handleLoadMore, sentinelRef, feeds.length, {
    throttleDelay: 200,
    rootMargin: "100px 0px",
    threshold: 0.1,
  });

  // Show skeleton loading state for immediate visual feedback
  if (isInitialLoading) {
    return (
      <Box minH="100dvh" position="relative">
        <Box
          p={5}
          maxW="container.sm"
          mx="auto"
          height="100dvh"
          data-testid="read-feeds-skeleton-container"
        >
          {/* Page Title */}
          <Box mb={6}>
            <Text
              fontSize="2xl"
              fontWeight="bold"
              color="var(--alt-primary)"
              textAlign="center"
              data-testid="read-feeds-title"
            >
              Viewed Feeds
            </Text>
          </Box>

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
    <Box minH="100dvh" position="relative">
      <Box
        aria-live="polite"
        aria-atomic="true"
        position="absolute"
        left="-10000px"
        width="1px"
        height="1px"
        overflow="hidden"
        clip="rect(0, 0, 0, 0)"
        visibility="hidden"
        whiteSpace="nowrap"
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
        data-testid="read-feeds-scroll-container"
      >
        {/* Page Title */}
        <Box mb={6}>
          <Text
            fontSize="2xl"
            fontWeight="bold"
            color="var(--alt-primary)"
            textAlign="center"
            data-testid="read-feeds-title"
          >
            Viewed Feeds
          </Text>
        </Box>

        {feeds.length > 0 ? (
          <>
            {/* Read Feed Cards */}
            <Flex direction="column" gap={4}>
              {feeds.map((feed: Feed) => (
                <ReadFeedCard key={feed.link} feed={feed} />
              ))}
            </Flex>

            {/* No more feeds indicator */}
            {!hasMore && feeds.length > 0 && (
              <Text
                textAlign="center"
                color="var(--alt-text-secondary)"
                fontSize="sm"
                mt={8}
                mb={4}
              >
                No more read feeds to load
              </Text>
            )}
          </>
        ) : (
          /* Empty state */
          <Flex justify="center" align="center" py={20}>
            <Box className="glass" p={8} borderRadius="18px" textAlign="center">
              <Text color="var(--alt-text-secondary)" fontSize="lg" mb={2}>
                No read feeds yet
              </Text>
              <Text color="var(--alt-text-secondary)" fontSize="sm">
                Mark some feeds as read to see them here
              </Text>
            </Box>
          </Flex>
        )}

        {/* Infinite scroll sentinel - always rendered when feeds are present and there's more to load */}
        {feeds.length > 0 && hasMore && (
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
                Loading more read feeds...
              </Text>
            )}
          </div>
        )}
      </Box>

      <FloatingMenu />
    </Box>
  );
}

export default function ReadFeedsPage() {
  return (
    <ViewedFeedsErrorBoundary>
      <ReadFeedsPageContent />
    </ViewedFeedsErrorBoundary>
  );
}

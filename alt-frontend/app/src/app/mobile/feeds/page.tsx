"use client";

import { Flex, Text, Button } from "@chakra-ui/react";
import { feedsApi } from "@/lib/api";
import { Feed } from "@/schema/feed";
import FeedCard from "@/components/mobile/FeedCard";
import { useEffect, useRef, useState, useCallback, useMemo } from "react";
import { useInfiniteScroll } from "@/lib/utils/infiniteScroll";
import ErrorState from "./_components/ErrorState";
import dynamic from "next/dynamic";

const PAGE_SIZE = 10;

const Progress = dynamic(
  () => import("@chakra-ui/progress").then((m) => m.CircularProgress),
  { ssr: false },
);

export default function Feeds() {
  const [feeds, setFeeds] = useState<Feed[]>([]);
  const [currentPage, setCurrentPage] = useState(0);
  const [isLoading, setIsLoading] = useState(false);
  const [hasMore, setHasMore] = useState(true);
  const [initialLoading, setInitialLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [readFeeds, setReadFeeds] = useState<Set<string>>(new Set());
  const [refreshKey, setRefreshKey] = useState<number>(0);
  const sentinelRef = useRef<HTMLDivElement>(null);

  // Memoize visible feeds to prevent unnecessary recalculations
  const visibleFeeds = useMemo(
    () => feeds.filter((feed) => !readFeeds.has(feed.link)),
    [feeds, readFeeds],
  );

  const loadInitialFeeds = useCallback(async () => {
    setInitialLoading(true);
    setError(null);

    try {
      let initialFeeds;
      try {
        initialFeeds = await feedsApi.getFeedsPage(0);
      } catch (pageError) {
        console.error("getFeedsPage failed, trying getAllFeeds:", pageError);
        try {
          const allFeeds = await feedsApi.getAllFeeds();
          initialFeeds = allFeeds.slice(0, PAGE_SIZE);
        } catch (allFeedsError) {
          console.error("getAllFeeds also failed:", allFeedsError);
          throw allFeedsError;
        }
      }

      // Batch state updates
      setFeeds(initialFeeds);
      setCurrentPage(0);
      setHasMore(initialFeeds.length === PAGE_SIZE);
    } catch (error) {
      console.error("Error fetching initial feeds:", error);
      const errorMessage =
        error instanceof Error ? error.message : "Failed to load feeds";

      // Batch error state updates
      setError(errorMessage);
      setFeeds([]);
      setHasMore(false);
    }
    setInitialLoading(false);
  }, []);

  useEffect(() => {
    loadInitialFeeds();
  }, [loadInitialFeeds]);

  const loadMore = useCallback(async () => {
    if (isLoading || !hasMore || error) {
      return;
    }

    setIsLoading(true);

    try {
      const nextPage = currentPage + 1;
      const newFeeds = await feedsApi.getFeedsPage(nextPage);

      if (newFeeds.length === 0) {
        setHasMore(false);
      } else {
        // Batch state updates
        setFeeds((prevFeeds) => [...prevFeeds, ...newFeeds]);
        setCurrentPage(nextPage);

        if (newFeeds.length < PAGE_SIZE) {
          setHasMore(false);
        }
      }
    } catch (loadError) {
      console.error("Error loading more feeds:", loadError);
      setHasMore(false);
    }

    setIsLoading(false);
  }, [currentPage, isLoading, hasMore, error]);

  // Use infinite scroll hook with reset key
  useInfiniteScroll(loadMore, sentinelRef, refreshKey);

  const handleMarkAsRead = useCallback((feedLink: string) => {
    setReadFeeds((prev) => new Set(prev).add(feedLink));
  }, []);

  const handleRefresh = useCallback(async () => {
    if (isLoading) return; // Prevent multiple refresh calls

    // Batch all state resets
    setIsLoading(true);
    setError(null);
    setReadFeeds(new Set()); // Clear read feeds to show all available feeds
    setCurrentPage(0);
    setHasMore(true);
    setFeeds([]);
    setRefreshKey((prev) => prev + 1); // Reset infinite scroll observer

    try {
      // Manually load fresh feeds instead of calling loadInitialFeeds to avoid conflicts
      let initialFeeds;
      try {
        initialFeeds = await feedsApi.getFeedsPage(0);
      } catch (pageError) {
        console.error("getFeedsPage failed, trying getAllFeeds:", pageError);
        try {
          const allFeeds = await feedsApi.getAllFeeds();
          initialFeeds = allFeeds.slice(0, PAGE_SIZE);
        } catch (allFeedsError) {
          console.error("getAllFeeds also failed:", allFeedsError);
          throw allFeedsError;
        }
      }

      // Batch successful state updates
      setFeeds(initialFeeds);
      setCurrentPage(0);
      setHasMore(initialFeeds.length === PAGE_SIZE);
    } catch (error) {
      console.error("Error refreshing feeds:", error);
      const errorMessage =
        error instanceof Error ? error.message : "Failed to refresh feeds";

      // Batch error state updates
      setError(errorMessage);
      setFeeds([]);
      setHasMore(false);
    }

    setIsLoading(false);
  }, [isLoading]);

  // Memoize loading component
  const LoadingComponent = useMemo(
    () => (
      <Flex
        flexDirection="column"
        justifyContent="center"
        alignItems="center"
        height="100vh"
        width="100%"
        data-testid="loading-spinner"
      >
        <Progress isIndeterminate color="indigo.500" size="md" />
      </Flex>
    ),
    [],
  );

  if (initialLoading) {
    return LoadingComponent;
  }

  if (error) {
    return (
      <ErrorState
        error={error}
        onRetry={loadInitialFeeds}
        isLoading={initialLoading}
      />
    );
  }

  return (
    <Flex
      flexDirection="column"
      alignItems="center"
      width="100%"
      bg="indigo.200"
      minHeight="100vh"
    >
      {visibleFeeds.length > 0 ? (
        <Flex
          flexDirection="column"
          alignItems="center"
          width="100%"
          bg={"whiteAlpha.200"}
        >
          {visibleFeeds.map((feed: Feed) => (
            <Flex
              key={feed.link}
              flexDirection="column"
              justifyContent="center"
              alignItems="center"
              width="100%"
              px={4}
              py={2}
            >
              <FeedCard
                feed={feed}
                isReadStatus={false}
                setIsReadStatus={() => handleMarkAsRead(feed.link)}
              />
            </Flex>
          ))}
          <div
            ref={sentinelRef}
            style={{
              height: "20px",
              width: "100%",
              backgroundColor: "transparent",
            }}
          />

          {isLoading && (
            <Flex justifyContent="center" p={4}>
              <Progress isIndeterminate color="indigo.500" size="md" />
            </Flex>
          )}

          {!hasMore && !isLoading && visibleFeeds.length === 0 && (
            <Flex
              flexDirection="column"
              justifyContent="center"
              alignItems="center"
              width="90%"
              p={4}
            >
              <Text color="white">No more feeds</Text>
              <Text color="gray.300" fontSize="sm" mt={2}>
                Try refreshing to see new content
              </Text>
            </Flex>
          )}
        </Flex>
      ) : (
        <Flex
          flexDirection="column"
          justifyContent="center"
          alignItems="center"
          height="100vh"
          width="100%"
        >
          <Text>No feeds available</Text>
        </Flex>
      )}

      {/* Refresh Button - Fixed position in bottom left corner */}
      <Button
        position="fixed"
        bottom="20px"
        left="20px"
        color="black"
        p={2}
        borderRadius="md"
        size="xl"
        fontSize="lg"
        onClick={handleRefresh}
        disabled={isLoading}
        zIndex={1000}
        boxShadow="lg"
      >
        {isLoading ? (
          <Progress
            isIndeterminate
            color="black"
            size="md"
            fontStyle={"italic"}
          />
        ) : (
          <Text color="ivory.200">Refresh</Text>
        )}
      </Button>
    </Flex>
  );
}

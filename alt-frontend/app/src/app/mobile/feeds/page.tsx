"use client";

import { Flex, Text, Button } from "@chakra-ui/react";
import { feedsApi } from "@/lib/api";
import { Feed } from "@/schema/feed";
import FeedCard from "@/component/mobile/FeedCard";
import { useEffect, useRef, useState, useCallback } from "react";
import { useInfiniteScroll } from "@/lib/utils/infiniteScroll";
import { CircularProgress } from "@chakra-ui/progress";

const PAGE_SIZE = 20;

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

  useEffect(() => {
    loadInitialFeeds();
  }, []);

  const loadInitialFeeds = async () => {
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

      setFeeds(initialFeeds);
      setCurrentPage(0);
      setHasMore(initialFeeds.length === PAGE_SIZE);
    } catch (error) {
      console.error("Error fetching initial feeds:", error);
      const errorMessage =
        error instanceof Error ? error.message : "Failed to load feeds";
      setError(errorMessage);
      setFeeds([]);
      setHasMore(false);
    }
    setInitialLoading(false);
  };

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

  // Create an InfiniteScrollWrapper component that remounts on refresh
  const InfiniteScrollWrapper = () => {
    useInfiniteScroll(loadMore, sentinelRef);
    return null;
  };

  const handleMarkAsRead = (feedLink: string) => {
    setReadFeeds((prev) => new Set(prev).add(feedLink));
  };

  const handleRefresh = async () => {
    if (isLoading) return; // Prevent multiple refresh calls

    setIsLoading(true);
    setError(null);
    setReadFeeds(new Set()); // Clear read feeds to show all available feeds

    // Reset all pagination-related states
    setCurrentPage(0);
    setHasMore(true);
    setFeeds([]);
    setRefreshKey(prev => prev + 1); // Force infinite scroll component to remount

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

      setFeeds(initialFeeds);
      setCurrentPage(0);
      setHasMore(initialFeeds.length === PAGE_SIZE);
    } catch (error) {
      console.error("Error refreshing feeds:", error);
      const errorMessage =
        error instanceof Error ? error.message : "Failed to refresh feeds";
      setError(errorMessage);
      setFeeds([]);
      setHasMore(false);
    }

    setIsLoading(false);
  };

  const visibleFeeds = feeds.filter((feed) => !readFeeds.has(feed.link));

  if (initialLoading) {
    return (
      <Flex
        flexDirection="column"
        justifyContent="center"
        alignItems="center"
        height="100vh"
        width="100%"
      >
        <CircularProgress isIndeterminate color="indigo.500" size="md" />
      </Flex>
    );
  }

  if (error) {
    return (
      <Flex
        flexDirection="column"
        justifyContent="center"
        alignItems="center"
        height="100vh"
        width="100%"
        p={4}
      >
        <Text
          fontSize="lg"
          fontWeight="bold"
          color="red.500"
          mb={4}
          textAlign="center"
        >
          Unable to load feeds
        </Text>
        <Text color="gray.600" mb={6} textAlign="center" maxWidth="md">
          {error}
        </Text>
        <Button
          colorScheme="indigo"
          onClick={loadInitialFeeds}
          disabled={initialLoading}
        >
          {initialLoading ? "Retrying..." : "Retry"}
        </Button>
      </Flex>
    );
  }

  return (
    <Flex
      flexDirection="column"
      alignItems="center"
      width="100%"
      bg="indigo.400"
      minHeight="100vh"
    >
      <InfiniteScrollWrapper key={refreshKey} />
      {visibleFeeds.length > 0 ? (
        <Flex flexDirection="column" alignItems="center" width="100%">
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
              <CircularProgress isIndeterminate color="indigo.500" size="md" />
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
        bg="ivory.200"
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
          <CircularProgress
            isIndeterminate
            color="black"
            size="md"
            fontStyle={"italic"}
          />
        ) : (
          "Refresh"
        )}
      </Button>
    </Flex>
  );
}

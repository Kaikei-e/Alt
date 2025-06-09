"use client";

import { Flex, Text, Button, Box } from "@chakra-ui/react";
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

  // Memoize loading component with vaporwave styling
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
        <Box p={6} borderRadius="20px" className="glass" textAlign="center">
          <Progress isIndeterminate color="pink.400" size="lg" />
          <Text mt={4} color="white" fontSize="lg" fontWeight="bold">
            Loading feeds...
          </Text>
        </Box>
      </Flex>
    ),
    [],
  );

  if (initialLoading) {
    return LoadingComponent;
  }

  if (error) {
    return (
      <Box minHeight="100vh" minH="100dvh">
        <ErrorState
          error={error}
          onRetry={loadInitialFeeds}
          isLoading={initialLoading}
        />
      </Box>
    );
  }

  return (
    <Box
      width="100%"
      className="feed-container"
      minHeight="100vh"
      minH="100dvh"
      position="relative"
    >
      <Flex
        flexDirection="column"
        alignItems="center"
        width="100%"
        px={4}
        pt={6}
        pb="calc(80px + env(safe-area-inset-bottom))"
      >
        {visibleFeeds.length > 0 ? (
          <>
            {visibleFeeds.map((feed: Feed) => (
              <Box
                key={feed.link}
                width="100%"
                maxWidth="500px"
                mb={4}
                position="relative"
              >
                {/* Gradient border effect */}
                <Box
                  position="absolute"
                  top="-2px"
                  left="-2px"
                  right="-2px"
                  bottom="-2px"
                  bg="linear-gradient(45deg, #ff006e, #8338ec, #3a86ff)"
                  borderRadius="16px"
                  zIndex={0}
                />
                <Box
                  position="relative"
                  zIndex={1}
                  bg="#1a1a2e"
                  borderRadius="14px"
                  overflow="hidden"
                  _hover={{
                    transform: "translateY(-2px)",
                  }}
                  transition="transform 0.2s ease"
                >
                  <FeedCard
                    feed={feed}
                    isReadStatus={false}
                    setIsReadStatus={() => handleMarkAsRead(feed.link)}
                  />
                </Box>
              </Box>
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
              <Flex justifyContent="center" p={6}>
                <Progress isIndeterminate color="blue.400" size="md" />
              </Flex>
            )}

            {!hasMore && !isLoading && visibleFeeds.length === 0 && (
              <Box
                p={6}
                borderRadius="16px"
                bg="rgba(255, 255, 255, 0.1)"
                border="1px solid rgba(255, 255, 255, 0.2)"
                textAlign="center"
                mt={8}
              >
                <Text color="white" fontSize="lg" fontWeight="bold">
                  No more feeds
                </Text>
                <Text color="gray.300" fontSize="sm" mt={2}>
                  Try refreshing to see new content
                </Text>
              </Box>
            )}
          </>
        ) : (
          <Flex
            flexDirection="column"
            justifyContent="center"
            alignItems="center"
            height="80vh"
            width="100%"
          >
            <Box
              p={8}
              borderRadius="20px"
              bg="rgba(255, 255, 255, 0.1)"
              border="1px solid rgba(255, 255, 255, 0.2)"
              textAlign="center"
            >
              <Text color="white" fontSize="xl" fontWeight="bold">
                No feeds available
              </Text>
            </Box>
          </Flex>
        )}
      </Flex>

      <Button
        position="fixed"
        bottom="calc(20px + env(safe-area-inset-bottom))"
        left="calc(20px + env(safe-area-inset-left))"
        size="lg"
        borderRadius="full"
        bg="linear-gradient(45deg, #ff006e, #8338ec)"
        color="white"
        fontWeight="bold"
        px={6}
        py={3}
        onClick={handleRefresh}
        disabled={isLoading}
        zIndex={1000}
        boxShadow="0 8px 32px rgba(255, 0, 110, 0.3)"
        border="2px solid rgba(255, 255, 255, 0.2)"
        _hover={{
          transform: "translateY(-2px)",
          bg: "linear-gradient(45deg, #e6005c, #7129d4)",
          boxShadow: "0 12px 40px rgba(255, 0, 110, 0.4)",
        }}
        _active={{
          transform: "translateY(0px)",
          boxShadow: "0 4px 20px rgba(255, 0, 110, 0.3)",
        }}
        _disabled={{
          opacity: 0.6,
          cursor: "not-allowed",
          _hover: {
            transform: "none",
            bg: "linear-gradient(45deg, #ff006e, #8338ec)",
          },
        }}
        transition="all 0.2s ease"
      >
        {isLoading ? (
          <Progress isIndeterminate color="white" size="sm" />
        ) : (
          "Refresh"
        )}
      </Button>
    </Box>
  );
}

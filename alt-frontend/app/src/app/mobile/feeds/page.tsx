"use client";

import { Flex, Text, Button } from "@chakra-ui/react";
import { feedsApi } from "@/lib/api";
import { Feed } from "@/schema/feed";
import FeedCard from "@/component/mobile/FeedCard";
import { useEffect, useRef, useState, useCallback } from "react";
import { useInfiniteScroll } from "@/lib/utils/infiniteScroll";
import { CircularProgress } from "@chakra-ui/progress";

const PAGE_SIZE = 20; // Backend uses 20 as page size

export default function Feeds() {
  const [feeds, setFeeds] = useState<Feed[]>([]);
  const [currentPage, setCurrentPage] = useState(0);
  const [isLoading, setIsLoading] = useState(false);
  const [hasMore, setHasMore] = useState(true);
  const [initialLoading, setInitialLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  // Load initial page
  useEffect(() => {
    loadInitialFeeds();
  }, []);

  const loadInitialFeeds = async () => {
    setInitialLoading(true);
    setError(null);

    try {
      // First test if backend is running
      const baseUrl = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:9000";
      console.log("Base URL:", baseUrl);
      console.log("Testing backend health...");

      console.log("Loading initial feeds from page 0...");
      let initialFeeds;
      try {
        console.log("Calling feedsApi.getFeedsPage(0)...");
        initialFeeds = await feedsApi.getFeedsPage(0);
        console.log("getFeedsPage succeeded, received feeds:", initialFeeds.length);
      } catch (pageError) {
        console.error("getFeedsPage failed, trying getAllFeeds:", pageError);
        try {
          console.log("Calling feedsApi.getAllFeeds()...");
          const allFeeds = await feedsApi.getAllFeeds();
          console.log("getAllFeeds succeeded, received feeds:", allFeeds.length);
          initialFeeds = allFeeds.slice(0, PAGE_SIZE);
        } catch (allFeedsError) {
          console.error("getAllFeeds also failed:", allFeedsError);
          throw allFeedsError;
        }
      }

      console.log("Received initial feeds:", initialFeeds.length, initialFeeds);
      setFeeds(initialFeeds);
      setCurrentPage(0);
      setHasMore(initialFeeds.length === PAGE_SIZE);
    } catch (error) {
      console.error("Error fetching initial feeds:", error);
      const errorMessage = error instanceof Error ? error.message : "Failed to load feeds";
      setError(errorMessage);
      setFeeds([]);
      setHasMore(false);
    }
    setInitialLoading(false);
  };

  const loadMore = useCallback(async () => {
    if (isLoading || !hasMore || error) return;

    setIsLoading(true);

    try {
      const nextPage = currentPage + 1;
      const newFeeds = await feedsApi.getFeedsPage(nextPage);

      if (newFeeds.length === 0) {
        // No more feeds available
        setHasMore(false);
      } else {
        // Append new feeds to existing ones
        setFeeds(prevFeeds => [...prevFeeds, ...newFeeds]);
        setCurrentPage(nextPage);

        // If we got less than PAGE_SIZE, this is the last page
        if (newFeeds.length < PAGE_SIZE) {
          setHasMore(false);
        }
      }
    } catch (error) {
      console.error("Error loading more feeds:", error);
      setHasMore(false);
    }

    setIsLoading(false);
  }, [currentPage, isLoading, hasMore, error]);

  useInfiniteScroll(loadMore, sentinelRef);

  if (initialLoading) {
    return (
      <Flex
        flexDirection="column"
        justifyContent="center"
        alignItems="center"
        height="100vh"
        width="100%"
      >
        <CircularProgress isIndeterminate color="indigo.500" size="lg" />
        <Text mt={4}>Loading feeds...</Text>
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
        <Text fontSize="lg" fontWeight="bold" color="red.500" mb={4} textAlign="center">
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
    <div style={{ minHeight: '100vh', width: '100%' }}>
      {feeds.length > 0 ? (
        <Flex
          flexDirection="column"
          alignItems="center"
          width="100%"
          gap={4}
          p={4}
        >
          {feeds.map((feed: Feed) => (
            <Flex
              key={feed.id}
              flexDirection="column"
              justifyContent="center"
              alignItems="center"
              width="90%"
              p={4}
            >
              <FeedCard feed={feed} />
            </Flex>
          ))}
          {isLoading && (
            <Flex justifyContent="center" p={4}>
              <CircularProgress isIndeterminate color="indigo.500" size="md" />
            </Flex>
          )}
          {/* Sentinel element for infinite scroll - only show when there's more to load */}
          {hasMore && !isLoading && (
            <div ref={sentinelRef} style={{ height: '20px', width: '100%' }} />
          )}
          {!hasMore && (
            <Flex
              flexDirection="column"
              justifyContent="center"
              alignItems="center"
              width="90%"
              p={4}
            >
              <Text>No more feeds</Text>
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
    </div>
  );
}

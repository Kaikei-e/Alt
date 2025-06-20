"use client";

import { Flex, Text, Box } from "@chakra-ui/react";
import { feedsApi } from "@/lib/api";
import { Feed } from "@/schema/feed";
import FeedCard from "@/components/mobile/FeedCard";
import { useEffect, useRef, useState, useCallback, useMemo } from "react";
import { useInfiniteScroll } from "@/lib/utils/infiniteScroll";
import { useCursorPagination } from "@/hooks/useCursorPagination";
import ErrorState from "./_components/ErrorState";
import dynamic from "next/dynamic";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";

const PAGE_SIZE = 20;

const Progress = dynamic(
  () => import("@chakra-ui/progress").then((m) => m.CircularProgress),
  { ssr: false },
);

export default function Feeds() {
  const [readFeeds, setReadFeeds] = useState<Set<string>>(new Set());
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

  useEffect(() => {
    loadInitial();
  }, [loadInitial]);

  // Use infinite scroll hook
  useInfiniteScroll(loadMore, sentinelRef);

  const handleMarkAsRead = useCallback((feedLink: string) => {
    setReadFeeds((prev) => new Set(prev).add(feedLink));
  }, []);

  if (isInitialLoading) {
    return (
      <Box minHeight="100vh" minH="100dvh" position="relative">
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
        <FloatingMenu />
      </Box>
    );
  }

  if (error) {
    return (
      <Box minHeight="100vh" minH="100dvh" position="relative">
        <ErrorState
          error={error}
          onRetry={refresh}
          isLoading={isInitialLoading}
        />
        <FloatingMenu />
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
                height: "100px",
                width: "100%",
                backgroundColor: "transparent",
                position: "relative",
                zIndex: 1,
              }}
            />

            {isLoading && (
              <Flex justifyContent="center" p={6} width="100%">
                <Box
                  p={4}
                  borderRadius="12px"
                  bg="rgba(255, 255, 255, 0.1)"
                  border="1px solid rgba(255, 255, 255, 0.2)"
                >
                  <Progress isIndeterminate color="blue.400" size="md" />
                  <Text color="white" fontSize="sm" mt={2} textAlign="center">
                    Loading more...
                  </Text>
                </Box>
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
      <FloatingMenu />
    </Box>
  );
}

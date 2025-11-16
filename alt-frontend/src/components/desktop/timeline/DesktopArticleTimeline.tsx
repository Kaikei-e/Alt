"use client";

import { Box, Flex, Text, VStack } from "@chakra-ui/react";
import React, { useCallback, useRef } from "react";
import { useCursorPagination } from "@/hooks/useCursorPagination";
import { articleApi } from "@/lib/api";
import type { Article } from "@/schema/article";
import { DesktopArticleCard } from "./DesktopArticleCard";

const PAGE_SIZE = 20;

const DesktopArticleTimeline = () => {
  // Using cursor-based pagination for articles
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  // Use cursor-based pagination hook
  const {
    data: articles,
    hasMore,
    isLoading,
    error,
    isInitialLoading,
    loadMore,
  } = useCursorPagination<Article>(articleApi.getArticlesWithCursor, {
    limit: PAGE_SIZE,
    autoLoad: true,
  });

  // Handle infinite scroll
  const handleLoadMore = useCallback(() => {
    if (hasMore && !isLoading) {
      loadMore();
    }
  }, [hasMore, isLoading, loadMore]);

  // Intersection Observer for infinite scroll
  React.useEffect(() => {
    const sentinelElement = sentinelRef.current;
    if (!sentinelElement) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting) {
          handleLoadMore();
        }
      },
      {
        root: scrollContainerRef.current,
        rootMargin: "200px",
        threshold: 0.1,
      },
    );

    observer.observe(sentinelElement);

    return () => {
      observer.disconnect();
    };
  }, [handleLoadMore]);

  // Show skeleton loading state
  if (isInitialLoading) {
    return (
      <Box w="100%" minH="0" flex={1} bg="var(--app-bg)">
        <Box overflowY="auto" h="100vh" p={6}>
          <Flex direction="column" gap={6} maxW="900px" mx="auto">
            {Array.from({ length: 3 }).map((_, index) => (
              <Box
                key={`skeleton-${index}`}
                className="glass"
                p={5}
                borderRadius="var(--radius-lg)"
                border="1px solid var(--surface-border)"
                opacity={0.6}
              >
                <VStack gap={3} align="stretch">
                  <Box
                    height="24px"
                    backgroundColor="var(--surface-bg)"
                    borderRadius="4px"
                    width="80%"
                  />
                  <Box
                    height="16px"
                    backgroundColor="var(--surface-bg)"
                    borderRadius="4px"
                    width="100%"
                  />
                  <Box
                    height="16px"
                    backgroundColor="var(--surface-bg)"
                    borderRadius="4px"
                    width="90%"
                  />
                </VStack>
              </Box>
            ))}
          </Flex>
        </Box>
      </Box>
    );
  }

  // Show error state
  if (error) {
    return (
      <Box w="100%" minH="0" flex={1} bg="var(--app-bg)">
        <Box
          display="flex"
          alignItems="center"
          justifyContent="center"
          h="100%"
          p={4}
        >
          <Box
            className="glass"
            p={6}
            borderRadius="var(--radius-lg)"
            textAlign="center"
            maxW="400px"
          >
            <Text fontSize="2xl" mb={3}>
              ⚠️
            </Text>
            <Text color="var(--text-primary)" fontSize="lg" mb={3}>
              記事の読み込みに失敗しました
            </Text>
            <Text color="var(--text-secondary)" fontSize="sm">
              {error.message}
            </Text>
          </Box>
        </Box>
      </Box>
    );
  }

  return (
    <Box w="100%" minH="0" flex={1} bg="var(--app-bg)">
      <Box
        ref={scrollContainerRef}
        overflowY="auto"
        h="100vh"
        p={6}
        css={{
          scrollBehavior: "smooth",
          "&::-webkit-scrollbar": {
            width: "6px",
          },
          "&::-webkit-scrollbar-track": {
            background: "var(--surface-secondary)",
            borderRadius: "3px",
          },
          "&::-webkit-scrollbar-thumb": {
            background: "var(--accent-primary)",
            borderRadius: "3px",
            opacity: 0.7,
          },
        }}
      >
        {articles && articles.length > 0 ? (
          <>
            <Flex direction="column" gap={5} maxW="900px" mx="auto">
              {articles.map((article) => (
                <DesktopArticleCard key={article.id} article={article} />
              ))}
            </Flex>

            {/* Infinite scroll sentinel */}
            {hasMore && (
              <Box
                ref={sentinelRef}
                h="60px"
                w="100%"
                mt={6}
                display="flex"
                alignItems="center"
                justifyContent="center"
              >
                {isLoading && (
                  <Text color="var(--text-secondary)" fontSize="sm">
                    読み込み中...
                  </Text>
                )}
              </Box>
            )}

            {/* No more articles indicator */}
            {!hasMore && articles.length > 0 && (
              <Box
                className="glass"
                p={4}
                borderRadius="var(--radius-lg)"
                maxW="900px"
                mx="auto"
                mt={4}
                textAlign="center"
              >
                <Text fontSize="lg" mb={1}>
                  📭
                </Text>
                <Text color="var(--text-secondary)" fontSize="sm">
                  すべての記事を表示しました
                </Text>
              </Box>
            )}
          </>
        ) : (
          <Flex justify="center" align="center" py={16} maxW="900px" mx="auto">
            <Box
              className="glass"
              p={6}
              borderRadius="var(--radius-lg)"
              textAlign="center"
            >
              <Text fontSize="2xl" mb={3}>
                📰
              </Text>
              <Text color="var(--text-primary)" fontSize="md" mb={2}>
                記事がありません
              </Text>
              <Text color="var(--text-secondary)" fontSize="sm">
                新しい記事が追加されるとここに表示されます
              </Text>
            </Box>
          </Flex>
        )}
      </Box>
    </Box>
  );
};

export { DesktopArticleTimeline };

"use client";

import { Box, Flex, Text } from "@chakra-ui/react";
import { useCallback, useRef, useState } from "react";
import ErrorState from "@/app/mobile/feeds/_components/ErrorState";
import { ArticleDetailsModal } from "@/components/mobile/articles/ArticleDetailsModal";
import { ArticleViewCard } from "@/components/mobile/articles/ArticleViewCard";
import SkeletonFeedCard from "@/components/mobile/SkeletonFeedCard";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { useCursorPagination } from "@/hooks/useCursorPagination";
import { articleApi } from "@/lib/api";
import { useInfiniteScroll } from "@/lib/utils/infiniteScroll";
import type { Article } from "@/schema/article";

const PAGE_SIZE = 20;

export default function ArticlesViewPage() {
  const [isRetrying, setIsRetrying] = useState(false);
  const [selectedArticle, setSelectedArticle] = useState<Article | null>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);
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
    refresh,
  } = useCursorPagination<Article>(articleApi.getArticlesWithCursor, {
    limit: PAGE_SIZE,
    autoLoad: true,
  });

  // Retry functionality
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

  // Handle infinite scroll
  const handleLoadMore = useCallback(() => {
    if (hasMore && !isLoading) {
      loadMore();
    }
  }, [hasMore, isLoading, loadMore]);

  useInfiniteScroll(handleLoadMore, sentinelRef, articles?.length || 0, {
    throttleDelay: 200,
    rootMargin: "100px 0px",
    threshold: 0.1,
  });

  // Handle article details click
  const handleDetailsClick = useCallback((article: Article) => {
    setSelectedArticle(article);
    setIsModalOpen(true);
  }, []);

  const handleCloseModal = useCallback(() => {
    setIsModalOpen(false);
    // Small delay before clearing selected article to allow modal to close smoothly
    setTimeout(() => setSelectedArticle(null), 300);
  }, []);

  // Show skeleton loading state
  if (isInitialLoading) {
    return (
      <Box minH="100dvh" position="relative">
        <Box
          p={5}
          maxW="container.sm"
          mx="auto"
          height="100dvh"
          data-testid="articles-skeleton-container"
        >
          <Text
            fontSize="2xl"
            fontWeight="bold"
            color="var(--alt-text-primary)"
            mb={6}
            textAlign="center"
          >
            Articles
          </Text>
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

  // Show error state
  if (error) {
    return <ErrorState error={error} onRetry={retryFetch} isLoading={isRetrying} />;
  }

  return (
    <Box minH="100dvh" position="relative">
      <Box
        ref={scrollContainerRef}
        p={5}
        maxW="container.sm"
        mx="auto"
        overflowY="auto"
        overflowX="hidden"
        height="100vh"
        data-testid="articles-scroll-container"
        bg="var(--app-bg)"
      >
        {/* Page Title */}
        <Text
          fontSize="2xl"
          fontWeight="bold"
          color="var(--alt-text-primary)"
          mb={6}
          textAlign="center"
          bgGradient="var(--accent-gradient)"
          bgClip="text"
        >
          Articles
        </Text>

        {articles && articles.length > 0 ? (
          <>
            {/* Article cards */}
            {articles.map((article) => (
              <ArticleViewCard
                key={article.id}
                article={article}
                onDetailsClick={() => handleDetailsClick(article)}
              />
            ))}

            {/* No more articles indicator */}
            {!hasMore && articles.length > 0 && (
              <Text
                textAlign="center"
                color="var(--alt-text-secondary)"
                fontSize="sm"
                mt={8}
                mb={4}
              >
                No more articles to load
              </Text>
            )}
          </>
        ) : (
          /* Empty state */
          <Box
            textAlign="center"
            py={12}
            px={4}
            bg="var(--alt-glass)"
            borderRadius="16px"
            border="1px solid var(--alt-glass-border)"
          >
            <Text fontSize="lg" color="var(--alt-text-primary)" fontWeight="semibold" mb={2}>
              No Articles Found
            </Text>
            <Text fontSize="sm" color="var(--alt-text-secondary)">
              There are no articles available at the moment.
            </Text>
          </Box>
        )}

        {/* Infinite scroll sentinel */}
        {articles && articles.length > 0 && hasMore && (
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

      {/* Article Details Modal */}
      {selectedArticle && (
        <ArticleDetailsModal
          article={selectedArticle}
          isOpen={isModalOpen}
          onClose={handleCloseModal}
        />
      )}

      <FloatingMenu />
    </Box>
  );
}

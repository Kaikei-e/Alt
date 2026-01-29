"use client";

import { Box, Flex, Text, useBreakpointValue, VStack } from "@chakra-ui/react";
import type React from "react";
import { memo, Suspense, useCallback, useMemo } from "react";
import { ErrorBoundary } from "react-error-boundary";
import { RightPanel } from "@/components/desktop/analytics/RightPanel";
import { DesktopFeedsLayout } from "@/components/desktop/layout/DesktopFeedsLayout";
import {
  DefaultSidebarProps,
  DesktopSidebar,
} from "@/components/desktop/layout/DesktopSidebar";
import { DesktopFeedCard } from "@/components/desktop/timeline/DesktopFeedCard";
import { desktopFeedsApi } from "@/lib/api/desktop-feeds";
import type { DesktopFeed, FilterState } from "@/types/desktop-feed";

interface OptimizedDesktopFeedsProps {
  feeds: DesktopFeed[];
  filters: FilterState;
  onFilterChange: (filters: FilterState) => void;
}

// メモ化されたフィードカード
const MemoizedFeedCard = memo(DesktopFeedCard);

// エラーフォールバック
const ErrorFallback = ({ error }: { error: unknown }) => (
  <Box
    className="glass"
    p={8}
    borderRadius="var(--radius-xl)"
    textAlign="center"
  >
    <Text color="var(--alt-error)" fontWeight="bold" mb={2}>
      An error occurred
    </Text>
    <Text color="var(--text-secondary)" fontSize="sm">
      {error instanceof Error ? error.message : "Unknown error"}
    </Text>
  </Box>
);

// スケルトンローダー
const SkeletonLoader = () => (
  <VStack gap={4}>
    {Array.from({ length: 3 }).map((_, i) => (
      <Box
        key={i}
        className="glass"
        p={6}
        borderRadius="var(--radius-xl)"
        w="full"
        h="200px"
      >
        <Flex direction="column" gap={4}>
          <Box
            h="20px"
            bg="var(--surface-border)"
            borderRadius="var(--radius-md)"
          />
          <Box
            h="16px"
            bg="var(--surface-border)"
            borderRadius="var(--radius-md)"
            w="80%"
          />
          <Box
            h="16px"
            bg="var(--surface-border)"
            borderRadius="var(--radius-md)"
            w="60%"
          />
        </Flex>
      </Box>
    ))}
  </VStack>
);

export const OptimizedDesktopFeeds: React.FC<OptimizedDesktopFeedsProps> = ({
  feeds,
  filters,
}) => {
  // レスポンシブ値
  const feedCardVariant = useBreakpointValue({
    base: "compact",
    md: "default",
    lg: "detailed",
  }) as "default" | "compact" | "detailed";

  // メモ化されたフィルタリング済みフィード
  const filteredFeeds = useMemo(() => {
    return feeds.filter((feed) => {
      if (
        filters.readStatus !== "all" &&
        (filters.readStatus === "read") !== feed.isRead
      ) {
        return false;
      }

      if (
        filters.sources.length > 0 &&
        !filters.sources.includes(feed.metadata.source.id)
      ) {
        return false;
      }

      if (
        filters.priority !== "all" &&
        feed.metadata.priority !== filters.priority
      ) {
        return false;
      }

      return true;
    });
  }, [feeds, filters]);

  // メモ化されたコールバック
  const handleMarkAsRead = useCallback(async (feedId: string) => {
    await desktopFeedsApi.markAsRead(feedId);
  }, []);

  const handleToggleFavorite = useCallback(
    async (feedId: string) => {
      const feed = feeds.find((f) => f.id === feedId);
      if (feed) {
        await desktopFeedsApi.toggleFavorite(feedId, !feed.isFavorited);
      }
    },
    [feeds],
  );

  const handleToggleBookmark = useCallback(
    async (feedId: string) => {
      const feed = feeds.find((f) => f.id === feedId);
      if (feed) {
        await desktopFeedsApi.toggleBookmark(feedId, !feed.isBookmarked);
      }
    },
    [feeds],
  );

  const handleReadLater = useCallback((feedId: string) => {}, []);

  const handleViewArticle = useCallback(
    (feedId: string) => {
      const feed = feeds.find((f) => f.id === feedId);
      if (feed) {
        window.open(feed.link, "_blank");
      }
    },
    [feeds],
  );

  return (
    <ErrorBoundary
      FallbackComponent={ErrorFallback}
      onError={(error) => console.error("Desktop feeds error:", error)}
    >
      <DesktopFeedsLayout
        sidebar={
          <Suspense fallback={<SkeletonLoader />}>
            <DesktopSidebar
              {...DefaultSidebarProps}
              isCollapsed={false}
              onToggleCollapse={() => {}}
            />
          </Suspense>
        }
      >
        <VStack gap="6" align="stretch">
          {filteredFeeds.map((feed) => (
            <MemoizedFeedCard
              key={feed.id}
              feed={feed}
              variant={feedCardVariant}
              onMarkAsRead={handleMarkAsRead}
              onToggleFavorite={handleToggleFavorite}
              onToggleBookmark={handleToggleBookmark}
              onReadLater={handleReadLater}
              onViewArticle={handleViewArticle}
            />
          ))}

          {filteredFeeds.length === 0 && (
            <Box
              className="glass"
              p={8}
              borderRadius="var(--radius-xl)"
              textAlign="center"
            >
              <Text fontSize="2xl" mb={2}>
                📭
              </Text>
              <Text color="var(--text-secondary)">No feeds available</Text>
            </Box>
          )}
        </VStack>
      </DesktopFeedsLayout>

      <Suspense fallback={<SkeletonLoader />}>
        <RightPanel />
      </Suspense>
    </ErrorBoundary>
  );
};

'use client';

import React, { Suspense, useMemo, useCallback, memo } from 'react';
import {
  Box,
  VStack,
  Text,
  Flex,
  useBreakpointValue
} from '@chakra-ui/react';
import { ErrorBoundary } from 'react-error-boundary';
import { DesktopFeedsLayout } from '@/components/desktop/layout/DesktopFeedsLayout';
import { DesktopFeedCard } from '@/components/desktop/timeline/DesktopFeedCard';
import { RightPanel } from '@/components/desktop/analytics/RightPanel';
import { DesktopHeader } from '@/components/desktop/layout/DesktopHeader';
import { DesktopSidebar } from '@/components/desktop/layout/DesktopSidebar';
import { FilterState, DesktopFeed } from '@/types/desktop-feed';
import { desktopFeedsApi } from '@/lib/api/desktop-feeds';

interface OptimizedDesktopFeedsProps {
  feeds: DesktopFeed[];
  filters: FilterState;
  onFilterChange: (filters: FilterState) => void;
}

// メモ化されたフィードカード
const MemoizedFeedCard = memo(DesktopFeedCard);

// エラーフォールバック
const ErrorFallback = ({ error }: { error: Error }) => (
  <Box className="glass" p={8} borderRadius="var(--radius-xl)" textAlign="center">
    <Text color="var(--alt-error)" fontWeight="bold" mb={2}>
      エラーが発生しました
    </Text>
    <Text color="var(--text-secondary)" fontSize="sm">
      {error.message}
    </Text>
  </Box>
);

// スケルトンローダー
const SkeletonLoader = () => (
  <VStack spacing={4}>
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
          <Box h="20px" bg="var(--surface-border)" borderRadius="var(--radius-md)" />
          <Box h="16px" bg="var(--surface-border)" borderRadius="var(--radius-md)" w="80%" />
          <Box h="16px" bg="var(--surface-border)" borderRadius="var(--radius-md)" w="60%" />
        </Flex>
      </Box>
    ))}
  </VStack>
);

export const OptimizedDesktopFeeds: React.FC<OptimizedDesktopFeedsProps> = ({
  feeds,
  filters,
  onFilterChange
}) => {
  // レスポンシブ値
  const feedCardVariant = useBreakpointValue({
    base: 'compact',
    md: 'default',
    lg: 'detailed'
  }) as 'default' | 'compact' | 'detailed';

  // メモ化されたフィルタリング済みフィード
  const filteredFeeds = useMemo(() => {
    return feeds.filter(feed => {
      if (filters.readStatus !== 'all' &&
          ((filters.readStatus === 'read') !== feed.isRead)) {
        return false;
      }

      if (filters.sources.length > 0 &&
          !filters.sources.includes(feed.metadata.source.id)) {
        return false;
      }

      if (filters.priority !== 'all' &&
          feed.metadata.priority !== filters.priority) {
        return false;
      }

      return true;
    });
  }, [feeds, filters]);

  // メモ化されたコールバック
  const handleMarkAsRead = useCallback(async (feedId: string) => {
    await desktopFeedsApi.markAsRead(feedId);
  }, []);

  const handleToggleFavorite = useCallback(async (feedId: string) => {
    const feed = feeds.find(f => f.id === feedId);
    if (feed) {
      await desktopFeedsApi.toggleFavorite(feedId, !feed.isFavorited);
    }
  }, [feeds]);

  const handleToggleBookmark = useCallback(async (feedId: string) => {
    const feed = feeds.find(f => f.id === feedId);
    if (feed) {
      await desktopFeedsApi.toggleBookmark(feedId, !feed.isBookmarked);
    }
  }, [feeds]);

  const handleReadLater = useCallback((feedId: string) => {
    console.log('Read later:', feedId);
  }, []);

  const handleViewArticle = useCallback((feedId: string) => {
    const feed = feeds.find(f => f.id === feedId);
    if (feed) {
      window.open(feed.link, '_blank');
    }
  }, [feeds]);

  return (
    <ErrorBoundary
      FallbackComponent={ErrorFallback}
      onError={(error) => console.error('Desktop feeds error:', error)}
    >
      <DesktopFeedsLayout
        header={<DesktopHeader />}
        sidebar={
          <Suspense fallback={<SkeletonLoader />}>
            <DesktopSidebar
              activeFilters={filters}
              onFilterChange={onFilterChange}
              feedSources={[]} // データを実際のAPIから取得
              isCollapsed={false}
              onToggleCollapse={() => {}}
            />
          </Suspense>
        }
      >
        <VStack spacing="6" align="stretch">
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
            <Box className="glass" p={8} borderRadius="var(--radius-xl)" textAlign="center">
              <Text fontSize="2xl" mb={2}>📭</Text>
              <Text color="var(--text-secondary)">
                フィードがありません
              </Text>
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
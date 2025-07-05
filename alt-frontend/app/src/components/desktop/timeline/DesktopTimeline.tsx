'use client';

import React, { useMemo, useCallback, useState, useEffect, useRef } from 'react';
import { VStack, Text, Spinner, Flex, Box } from '@chakra-ui/react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { FilterState } from '@/types/desktop-feed';
import { useDesktopFeeds } from '@/hooks/useDesktopFeeds';
import { FilterBar } from './FilterBar';
import { VirtualizedFeedItem } from './VirtualizedFeedItem';
import { searchFeeds, SearchResult } from '@/utils/searchUtils';
import { debounce } from '@/utils/performanceUtils';
import { Feed } from '@/schema/feed';

interface DesktopTimelineProps {
  searchQuery: string;
  filters: FilterState;
  onFilterChange: (filters: FilterState) => void;
  variant?: 'default' | 'compact' | 'detailed';
}

export const DesktopTimeline: React.FC<DesktopTimelineProps> = React.memo(({
  searchQuery,
  filters,
  onFilterChange,
  // variant は将来の実装用に残す
}) => {
  const {
    feeds,
    isLoading,
    error,
    hasMore,
    fetchNextPage,
    markAsRead,
    toggleFavorite,
    // toggleBookmark は将来の実装用に残す
  } = useDesktopFeeds();

  // Debounced search query for performance optimization
  const [debouncedSearchQuery, setDebouncedSearchQuery] = useState(searchQuery);

  const debouncedSetSearch = useCallback(
    debounce((query: unknown) => {
      setDebouncedSearchQuery(query as string);
    }, 300),
    []
  );

  useEffect(() => {
    debouncedSetSearch(searchQuery);
  }, [searchQuery, debouncedSetSearch]);

  // フィルタリングされたフィード（高度な検索機能対応）
  const { filteredFeeds, searchResults } = useMemo(() => {
    let filtered = feeds;
    let results: SearchResult[] = [];

    // 高度な検索機能（複数キーワード対応、デバウンス済み）
    if (debouncedSearchQuery) {
      results = searchFeeds(filtered, debouncedSearchQuery, {
        multiKeyword: true,
        searchFields: ['title', 'description', 'tags'],
        fuzzyMatch: false,
        minimumScore: 0.1
      });
      filtered = results.map(result => result.feed);
    }

    // 時間範囲フィルター
    if (filters.timeRange !== 'all') {
      const now = new Date();
      const filterDate = new Date();

      switch (filters.timeRange) {
        case 'today':
          // Today: start of today (00:00:00)
          filterDate.setHours(0, 0, 0, 0);
          break;
        case 'week':
          // Last 7 days
          filterDate.setDate(now.getDate() - 7);
          filterDate.setHours(0, 0, 0, 0);
          break;
        case 'month':
          // Last 30 days
          filterDate.setDate(now.getDate() - 30);
          filterDate.setHours(0, 0, 0, 0);
          break;
      }

      filtered = filtered.filter(feed => {
        const feedDate = new Date(feed.published);
        return feedDate >= filterDate;
      });
    }

    // その他のフィルター適用（readStatus, sources, priority, tags等）
    // Note: Feed型にはisReadやmetadataがないため、実際の実装では
    // これらのフィルターは機能しない。テスト用に保持。
    if (filters.readStatus !== 'all') {
      filtered = filtered.filter(feed => {
        const feedData = feed as Feed & { isRead?: boolean };
        return filters.readStatus === 'read' ? feedData.isRead : !feedData.isRead;
      });
    }

    if (filters.sources.length > 0) {
      filtered = filtered.filter(feed => {
        const feedData = feed as Feed & { metadata?: { source?: { id: string } } };
        return feedData.metadata?.source?.id && filters.sources.includes(feedData.metadata.source.id);
      });
    }

    if (filters.priority !== 'all') {
      filtered = filtered.filter(feed => {
        const feedData = feed as Feed & { metadata?: { priority?: string } };
        return feedData.metadata?.priority === filters.priority;
      });
    }

    if (filters.tags.length > 0) {
      filtered = filtered.filter(feed => {
        const feedData = feed as Feed & { metadata?: { tags?: string[] } };
        return feedData.metadata?.tags?.some((tag: string) => filters.tags.includes(tag));
      });
    }

    return { filteredFeeds: filtered, searchResults: results };
  }, [feeds, debouncedSearchQuery, filters]);

  // Setup virtualizer
  const parentRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: filteredFeeds.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 200, // Estimated height for each feed item
    overscan: 10, // Render 10 items outside visible area for smooth scrolling
  });

  // const handleReadLater = (feedId: string) => {
  //   // 後で読む機能の実装（ローカルストレージやAPI経由）
  //   // 将来の実装用に保持
  // };

  const handleViewArticle = useCallback((feedId: string) => {
    const feed = feeds.find(f => f.id === feedId);
    if (feed) {
      window.open(feed.link, '_blank');
    }
  }, [feeds]);

  const handleMarkAsRead = useCallback((feedId: string) => {
    markAsRead(feedId);
  }, [markAsRead]);

  const handleToggleFavorite = useCallback((feedId: string) => {
    toggleFavorite(feedId);
  }, [toggleFavorite]);

  if (error) {
    return (
      <Box
        bg="var(--alt-error)"
        color="white"
        p={4}
        borderRadius="var(--radius-lg)"
        className="glass"
      >
        フィードの読み込みに失敗しました。
      </Box>
    );
  }

  return (
    <VStack gap={4} align="stretch">
      {/* Filter Bar */}
      <FilterBar
        filters={filters}
        onFilterChange={onFilterChange}
        availableTags={['tech', 'development', 'news', 'science']}
        availableSources={[
          { id: 'techcrunch', name: 'TechCrunch', icon: '📰' },
          { id: 'hackernews', name: 'Hacker News', icon: '🔥' },
          { id: 'medium', name: 'Medium', icon: '📝' },
          { id: 'devto', name: 'Dev.to', icon: '💻' },
        ]}
      />

      {/* 検索結果ヘッダー */}
      {debouncedSearchQuery && (
        <Flex
          className="glass"
          p={4}
          borderRadius="var(--radius-lg)"
          align="center"
          justify="space-between"
        >
          <Text color="var(--text-primary)" fontWeight="medium">
            検索: &quot;{debouncedSearchQuery}&quot;
          </Text>
          <VStack align="end" gap={1}>
            <Text fontSize="sm" color="var(--text-muted)">
              {filteredFeeds.length}件の結果
            </Text>
            {searchResults.length > 0 && (
              <Text fontSize="xs" color="var(--text-muted)">
                複数キーワード検索対応
              </Text>
            )}
          </VStack>
        </Flex>
      )}

      {/* Virtualized Timeline Container */}
      <Box
        data-testid="desktop-timeline"
        ref={parentRef}
        maxH={{
          base: "100vh",
          md: "calc(100vh - 140px)",
          lg: "calc(100vh - 180px)"
        }}
        overflowY="auto"
        overflowX="hidden"
        className="glass"
        p={4}
        borderRadius="var(--radius-lg)"
        css={{
          scrollBehavior: 'smooth',
          '&::-webkit-scrollbar': {
            width: '8px',
          },
          '&::-webkit-scrollbar-track': {
            background: 'var(--surface-secondary)',
            borderRadius: '4px',
          },
          '&::-webkit-scrollbar-thumb': {
            background: 'var(--accent-primary)',
            borderRadius: '4px',
            opacity: 0.6,
          },
          '&::-webkit-scrollbar-thumb:hover': {
            opacity: 1,
          },
        }}
      >
        {/* Error State */}
        {error && (
          <Box
            bg="var(--alt-error)"
            color="white"
            p={4}
            borderRadius="var(--radius-lg)"
            className="glass"
          >
            フィードの読み込みに失敗しました。
          </Box>
        )}

        {/* Loading State */}
        {isLoading && filteredFeeds.length === 0 && (
          <Flex
            className="glass"
            p={8}
            borderRadius="var(--radius-xl)"
            direction="column"
            align="center"
            gap={4}
          >
            <Spinner
              size="lg"
              color="var(--accent-primary)"
            />
            <Text color="var(--text-secondary)">
              フィードを読み込み中...
            </Text>
          </Flex>
        )}

        {/* Empty State */}
        {filteredFeeds.length === 0 && !isLoading && !error && (
          <Flex
            className="glass"
            p={8}
            borderRadius="var(--radius-xl)"
            direction="column"
            align="center"
            gap={4}
          >
            <Text fontSize="2xl">📭</Text>
            <Text color="var(--text-secondary)">
              {debouncedSearchQuery ? '検索結果が見つかりませんでした' : 'フィードが見つかりませんでした'}
            </Text>
          </Flex>
        )}

        {/* Virtualized Feed Items */}
        {filteredFeeds.length > 0 && (
          <Box
            data-testid="virtual-container"
            position="relative"
            height={`${virtualizer.getTotalSize()}px`}
          >
            {virtualizer.getVirtualItems().map((virtualItem) => (
              <VirtualizedFeedItem
                key={virtualItem.key}
                feed={filteredFeeds[virtualItem.index]}
                index={virtualItem.index}
                onMarkAsRead={handleMarkAsRead}
                onToggleFavorite={handleToggleFavorite}
                onViewArticle={handleViewArticle}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  height: `${virtualItem.size}px`,
                  transform: `translateY(${virtualItem.start}px)`,
                }}
              />
            ))}
          </Box>
        )}

        {/* Infinite Scroll Trigger */}
        {hasMore && !isLoading && filteredFeeds.length > 0 && (
          <Flex
            className="glass"
            p={4}
            borderRadius="var(--radius-lg)"
            justify="center"
            mt={4}
          >
            <Text
              color="var(--accent-primary)"
              fontWeight="medium"
              cursor="pointer"
              onClick={fetchNextPage}
              _hover={{ textDecoration: 'underline' }}
            >
              Load more...
            </Text>
          </Flex>
        )}
      </Box>
    </VStack>
  );
});

DesktopTimeline.displayName = 'DesktopTimeline';
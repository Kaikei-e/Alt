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
import { DesktopFeed } from '@/types/desktop-feed';

interface DesktopTimelineProps {
  searchQuery: string;
  filters: FilterState;
  onFilterChange: (filters: FilterState) => void;
  onSearchClear?: () => void;
  variant?: 'default' | 'compact' | 'detailed';
}

export const DesktopTimeline: React.FC<DesktopTimelineProps> = React.memo(({
  searchQuery,
  filters,
  onFilterChange,
  onSearchClear,
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
    toggleBookmark,
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
      filtered = results.map(result => result.feed as DesktopFeed);
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

    // その他のフィルター適用
    if (filters.readStatus !== 'all') {
      filtered = filtered.filter(feed => {
        return filters.readStatus === 'read' ? feed.isRead : !feed.isRead;
      });
    }

    if (filters.sources.length > 0) {
      filtered = filtered.filter(feed => {
        return feed.metadata?.source?.id && filters.sources.includes(feed.metadata.source.id);
      });
    }

    if (filters.priority !== 'all') {
      filtered = filtered.filter(feed => {
        return feed.metadata?.priority === filters.priority;
      });
    }

    if (filters.tags.length > 0) {
      filtered = filtered.filter(feed => {
        return feed.metadata?.tags?.some((tag: string) => filters.tags.includes(tag));
      });
    }

    return { filteredFeeds: filtered, searchResults: results };
  }, [feeds, debouncedSearchQuery, filters]);

  // Setup virtualizer and infinite scroll
  const parentRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: filteredFeeds.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 200, // Estimated height for each feed item
    overscan: 10, // Render 10 items outside visible area for smooth scrolling
  });

  // Infinite scroll implementation
  useEffect(() => {
    const container = parentRef.current;
    if (!container) return;

    const handleScroll = () => {
      const { scrollTop, scrollHeight, clientHeight } = container;
      const isNearBottom = scrollTop + clientHeight >= scrollHeight - 100;

      if (isNearBottom && hasMore && !isLoading) {
        fetchNextPage();
      }
    };

    container.addEventListener('scroll', handleScroll);
    return () => container.removeEventListener('scroll', handleScroll);
  }, [hasMore, isLoading, fetchNextPage]);

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

  const handleToggleBookmark = useCallback((feedId: string) => {
    toggleBookmark(feedId);
  }, [toggleBookmark]);

  // Remove early return for error to allow component to render

  return (
    <VStack gap={4} align="stretch" flex={1} h="stretch">
      {/* Filter Bar */}
      <FilterBar
        data-testid="filter-bar"
        filters={filters}
        onFilterChange={onFilterChange}
        onSearchClear={onSearchClear}
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
            Search: &quot;{debouncedSearchQuery}&quot;
          </Text>
          <VStack align="end" gap={1}>
            <Text fontSize="sm" color="var(--text-muted)">
              {filteredFeeds.length} results
            </Text>
            {searchResults.length > 0 && (
              <Text fontSize="xs" color="var(--text-muted)">
                Multi-keyword search enabled
              </Text>
            )}
          </VStack>
        </Flex>
      )}

      {/* Virtualized Timeline Container */}
      <Box
        data-testid="desktop-timeline"
        ref={parentRef}
        flex={1}
        minH="stretch"
        overflowY="scroll"
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
        {error && filteredFeeds.length === 0 && (
          <Flex
            className="glass"
            p={8}
            borderRadius="var(--radius-xl)"
            direction="column"
            align="center"
            gap={4}
          >
            <Text fontSize="2xl">⚠️</Text>
            <Text color="var(--text-secondary)">
              Failed to load feeds.
            </Text>
            <Text fontSize="sm" color="var(--text-muted)">
              {error.message}
            </Text>
          </Flex>
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
              Loading feeds...
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
              {debouncedSearchQuery ? 'No search results found' : 'No feeds found'}
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
                onToggleBookmark={handleToggleBookmark}
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

        {/* Loading indicator for infinite scroll */}
        {isLoading && filteredFeeds.length > 0 && (
          <Flex
            className="glass"
            p={4}
            borderRadius="var(--radius-lg)"
            justify="center"
            mt={4}
          >
            <Spinner
              size="sm"
              color="var(--accent-primary)"
            />
            <Text ml={2} color="var(--text-secondary)">
              Loading more feeds...
            </Text>
          </Flex>
        )}
      </Box>
    </VStack>
  );
});

DesktopTimeline.displayName = 'DesktopTimeline';
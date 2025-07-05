'use client';

import React, { useMemo } from 'react';
import { VStack, Text, Spinner, Flex, Box } from '@chakra-ui/react';
import { DesktopFeedCard } from './DesktopFeedCard';
import { FilterState } from '@/types/desktop-feed';
import { useDesktopFeeds } from '@/hooks/useDesktopFeeds';

interface DesktopTimelineProps {
  searchQuery: string;
  filters: FilterState;
  variant?: 'default' | 'compact' | 'detailed';
}

export const DesktopTimeline: React.FC<DesktopTimelineProps> = ({
  searchQuery,
  filters,
  variant = 'default'
}) => {
  const {
    feeds,
    isLoading,
    error,
    hasMore,
    fetchNextPage,
    markAsRead,
    toggleFavorite,
    toggleBookmark
  } = useDesktopFeeds();

  // フィルタリングされたフィード
  const filteredFeeds = useMemo(() => {
    let filtered = feeds;

    // 検索クエリフィルター
    if (searchQuery) {
      filtered = filtered.filter(feed =>
        feed.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
        feed.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
        feed.metadata.tags.some(tag =>
          tag.toLowerCase().includes(searchQuery.toLowerCase())
        )
      );
    }

    // 読書状態フィルター
    if (filters.readStatus !== 'all') {
      filtered = filtered.filter(feed =>
        filters.readStatus === 'read' ? feed.isRead : !feed.isRead
      );
    }

    // ソースフィルター
    if (filters.sources.length > 0) {
      filtered = filtered.filter(feed =>
        filters.sources.includes(feed.metadata.source.id)
      );
    }

    // 優先度フィルター
    if (filters.priority !== 'all') {
      filtered = filtered.filter(feed =>
        feed.metadata.priority === filters.priority
      );
    }

    // タグフィルター
    if (filters.tags.length > 0) {
      filtered = filtered.filter(feed =>
        filters.tags.some(tag => feed.metadata.tags.includes(tag))
      );
    }

    // 時間範囲フィルター
    if (filters.timeRange !== 'all') {
      const now = new Date();
      const filterDate = new Date();

      switch (filters.timeRange) {
        case 'today':
          filterDate.setDate(now.getDate());
          break;
        case 'week':
          filterDate.setDate(now.getDate() - 7);
          break;
        case 'month':
          filterDate.setMonth(now.getMonth() - 1);
          break;
      }

      filtered = filtered.filter(feed =>
        new Date(feed.published) >= filterDate
      );
    }

    return filtered;
  }, [feeds, searchQuery, filters]);

  const handleReadLater = (feedId: string) => {
    // 後で読む機能の実装（ローカルストレージやAPI経由）
    console.log('Read later:', feedId);
  };

  const handleViewArticle = (feedId: string) => {
    const feed = feeds.find(f => f.id === feedId);
    if (feed) {
      window.open(feed.link, '_blank');
    }
  };

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
    <Box
      data-testid="desktop-timeline"
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
      <VStack gap={4} align="stretch">
        {/* 検索結果ヘッダー */}
        {searchQuery && (
          <Flex
            className="glass"
            p={4}
            borderRadius="var(--radius-lg)"
            align="center"
            justify="space-between"
          >
            <Text color="var(--text-primary)" fontWeight="medium">
              検索: &quot;{searchQuery}&quot;
            </Text>
            <Text fontSize="sm" color="var(--text-muted)">
              {filteredFeeds.length}件の結果
            </Text>
          </Flex>
        )}

        {/* フィードカード一覧 */}
        {filteredFeeds.map((feed) => (
          <DesktopFeedCard
            key={feed.id}
            feed={feed}
            variant={variant}
            onMarkAsRead={markAsRead}
            onToggleFavorite={toggleFavorite}
            onToggleBookmark={toggleBookmark}
            onReadLater={handleReadLater}
            onViewArticle={handleViewArticle}
          />
        ))}

        {/* ローディング状態 */}
        {isLoading && (
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

        {/* 空の状態 */}
        {filteredFeeds.length === 0 && !isLoading && (
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
              {searchQuery ? '検索結果が見つかりませんでした' : 'フィードカードはTASK2で実装されます'}
            </Text>
          </Flex>
        )}

        {/* 無限スクロール用トリガー */}
        {hasMore && !isLoading && filteredFeeds.length > 0 && (
          <Flex
            className="glass"
            p={4}
            borderRadius="var(--radius-lg)"
            justify="center"
          >
            <Text
              color="var(--accent-primary)"
              fontWeight="medium"
              cursor="pointer"
              onClick={fetchNextPage}
              _hover={{ textDecoration: 'underline' }}
            >
              さらに読み込む
            </Text>
          </Flex>
        )}
      </VStack>
    </Box>
  );
};
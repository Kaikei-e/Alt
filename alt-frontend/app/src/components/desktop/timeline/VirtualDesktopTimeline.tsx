"use client";

import { useVirtualizer } from '@tanstack/react-virtual';
import { useRef, useCallback, useMemo } from 'react';
import { Box, Flex, Text } from '@chakra-ui/react';
import { Feed } from '@/schema/feed';
import { DesktopFeed } from '@/types/desktop-feed';
import { DesktopFeedCard } from './DesktopFeedCard';

interface VirtualDesktopTimelineProps {
  feeds: Feed[];
  readFeeds: Set<string>;
  onMarkAsRead: (feedId: string) => void;
  onToggleFavorite: (feedId: string) => void;
  onToggleBookmark: (feedId: string) => void;
  onReadLater: (feedId: string) => void;
  onViewArticle: (feedId: string) => void;
  height?: number;
}

// Transform Feed to DesktopFeed
const transformFeedToDesktopFeed = (feed: Feed): DesktopFeed => {
  return {
    ...feed,
    metadata: {
      source: {
        id: "rss",
        name: "RSS Feed",
        icon: "📰",
        reliability: 0.8,
        category: "general",
        unreadCount: 0,
        avgReadingTime: 5,
      },
      readingTime: Math.max(1, Math.ceil(feed.description.length / 200)),
      engagement: {
        likes: Math.floor(Math.random() * 50),
        bookmarks: Math.floor(Math.random() * 20),
      },
      tags: [],
      relatedCount: 0,
      publishedAt: feed.published,
      priority: "medium" as const,
      category: "general",
      difficulty: "intermediate" as const,
      summary: feed.description.substring(0, 200) + "...",
    },
    isRead: false,
    isFavorited: false,
    isBookmarked: false,
  };
};

export const VirtualDesktopTimeline: React.FC<VirtualDesktopTimelineProps> = ({
  feeds,
  readFeeds,
  onMarkAsRead,
  onToggleFavorite,
  onToggleBookmark,
  onReadLater,
  onViewArticle,
  height = 800,
}) => {
  const parentRef = useRef<HTMLDivElement>(null);

  // Transform feeds to desktop format
  const desktopFeeds = useMemo(() => {
    return feeds?.map(transformFeedToDesktopFeed) || [];
  }, [feeds]);

  // Filter out read feeds
  const visibleFeeds = useMemo(
    () => desktopFeeds.filter(feed => !readFeeds.has(feed.id)),
    [desktopFeeds, readFeeds]
  );

  // Create virtualizer with larger estimated size for desktop cards
  const virtualizer = useVirtualizer({
    count: visibleFeeds.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 420, // Estimated height of DesktopFeedCard
    overscan: 3, // Render 3 items outside of visible area (desktop cards are larger)
  });

  // Handle actions
  const handleMarkAsRead = useCallback((feedId: string) => {
    onMarkAsRead(feedId);
  }, [onMarkAsRead]);

  const handleToggleFavorite = useCallback((feedId: string) => {
    onToggleFavorite(feedId);
  }, [onToggleFavorite]);

  const handleToggleBookmark = useCallback((feedId: string) => {
    onToggleBookmark(feedId);
  }, [onToggleBookmark]);

  const handleReadLater = useCallback((feedId: string) => {
    onReadLater(feedId);
  }, [onReadLater]);

  const handleViewArticle = useCallback((feedId: string) => {
    onViewArticle(feedId);
  }, [onViewArticle]);

  // Don't render if no feeds
  if (visibleFeeds.length === 0) {
    return (
      <Flex justify="center" align="center" py={16} maxW="1000px" mx="auto">
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
            No feeds available
          </Text>
          <Text color="var(--text-secondary)" fontSize="sm">
            Your feed will appear here once you subscribe to sources
          </Text>
        </Box>
      </Flex>
    );
  }

  return (
    <Box
      ref={parentRef}
      data-testid="virtual-desktop-timeline"
      height={`${height}px`}
      overflowY="auto"
      overflowX="hidden"
      px={6}
      css={{
        scrollBehavior: 'smooth',
        '&::-webkit-scrollbar': {
          width: '6px',
        },
        '&::-webkit-scrollbar-track': {
          background: 'var(--surface-secondary)',
          borderRadius: '3px',
        },
        '&::-webkit-scrollbar-thumb': {
          background: 'var(--accent-primary)',
          borderRadius: '3px',
          opacity: 0.7,
        },
        '&::-webkit-scrollbar-thumb:hover': {
          opacity: 1,
        },
      }}
    >
      <Box
        height={`${virtualizer.getTotalSize()}px`}
        width="100%"
        position="relative"
        maxW="1000px"
        mx="auto"
      >
        {virtualizer.getVirtualItems().map((virtualItem) => {
          const feed = visibleFeeds[virtualItem.index];
          
          return (
            <Box
              key={virtualItem.key}
              data-testid={`virtual-desktop-item-${virtualItem.index}`}
              position="absolute"
              top={0}
              left={0}
              width="100%"
              height={`${virtualItem.size}px`}
              transform={`translateY(${virtualItem.start}px)`}
              py={3}
            >
              <DesktopFeedCard
                feed={feed}
                variant="default"
                onMarkAsRead={handleMarkAsRead}
                onToggleFavorite={handleToggleFavorite}
                onToggleBookmark={handleToggleBookmark}
                onReadLater={handleReadLater}
                onViewArticle={handleViewArticle}
              />
            </Box>
          );
        })}
      </Box>
    </Box>
  );
};

export default VirtualDesktopTimeline;
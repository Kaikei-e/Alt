"use client";

import { useVirtualizer } from '@tanstack/react-virtual';
import { useRef, useCallback, useMemo } from 'react';
import { Box, Flex } from '@chakra-ui/react';
import { Feed } from '@/schema/feed';
import FeedCard from './FeedCard';

interface VirtualFeedListProps {
  feeds: Feed[];
  readFeeds: Set<string>;
  onMarkAsRead: (feedLink: string) => void;
  height?: number;
}

export const VirtualFeedList: React.FC<VirtualFeedListProps> = ({
  feeds,
  readFeeds,
  onMarkAsRead,
  height = 600,
}) => {
  const parentRef = useRef<HTMLDivElement>(null);

  // Filter out read feeds
  const visibleFeeds = useMemo(
    () => feeds.filter(feed => !readFeeds.has(feed.link)),
    [feeds, readFeeds]
  );

  // Create virtualizer
  const virtualizer = useVirtualizer({
    count: visibleFeeds.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 280, // Estimated height of FeedCard
    overscan: 5, // Render 5 items outside of visible area
  });

  // Handle marking feed as read
  const handleMarkAsRead = useCallback((feedLink: string) => {
    onMarkAsRead(feedLink);
  }, [onMarkAsRead]);

  // Don't render if no feeds
  if (visibleFeeds.length === 0) {
    return null;
  }

  return (
    <Box
      ref={parentRef}
      data-testid="virtual-feed-list"
      height={`${height}px`}
      overflowY="auto"
      overflowX="hidden"
      css={{
        scrollBehavior: 'smooth',
        '&::-webkit-scrollbar': {
          width: '4px',
        },
        '&::-webkit-scrollbar-track': {
          background: 'var(--surface-secondary)',
          borderRadius: '2px',
        },
        '&::-webkit-scrollbar-thumb': {
          background: 'var(--accent-primary)',
          borderRadius: '2px',
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
      >
        {virtualizer.getVirtualItems().map((virtualItem) => {
          const feed = visibleFeeds[virtualItem.index];
          
          return (
            <Box
              key={virtualItem.key}
              data-testid={`virtual-feed-item-${virtualItem.index}`}
              position="absolute"
              top={0}
              left={0}
              width="100%"
              height={`${virtualItem.size}px`}
              transform={`translateY(${virtualItem.start}px)`}
              px={5}
              py={2}
            >
              <FeedCard
                feed={feed}
                isReadStatus={readFeeds.has(feed.link)}
                setIsReadStatus={() => handleMarkAsRead(feed.link)}
              />
            </Box>
          );
        })}
      </Box>
    </Box>
  );
};

export default VirtualFeedList;
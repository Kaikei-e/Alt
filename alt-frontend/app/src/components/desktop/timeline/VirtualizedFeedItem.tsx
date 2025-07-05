'use client';

import React, { memo } from 'react';
import { Box, VStack, Text, Flex } from '@chakra-ui/react';
import { DesktopFeed } from '@/types/desktop-feed';

interface VirtualizedFeedItemProps {
  feed: DesktopFeed;
  index: number;
  onMarkAsRead: (feedId: string) => void;
  onToggleFavorite: (feedId: string) => void;
  onToggleBookmark?: (feedId: string) => void;
  onViewArticle: (feedId: string) => void;
  style?: React.CSSProperties;
}

export const VirtualizedFeedItem: React.FC<VirtualizedFeedItemProps> = memo(({
  feed,
  index,
  onMarkAsRead,
  onToggleFavorite,
  onToggleBookmark,
  onViewArticle,
  style,
}) => {
  const handleViewArticle = () => {
    onViewArticle(feed.id);
  };

  const handleMarkAsRead = (e: React.MouseEvent) => {
    e.stopPropagation();
    onMarkAsRead(feed.id);
  };

  const handleToggleFavorite = (e: React.MouseEvent) => {
    e.stopPropagation();
    onToggleFavorite(feed.id);
  };

  const handleToggleBookmark = (e: React.MouseEvent) => {
    e.stopPropagation();
    onToggleBookmark?.(feed.id);
  };

  return (
    <Box
      data-testid={`feed-item-${index}`}
      style={style}
      position="absolute"
      top={0}
      left={0}
      right={0}
      p={2}
    >
      <Box
        className="glass"
        p={5}
        borderRadius="var(--radius-lg)"
        _hover={{
          transform: 'translateY(-2px)',
          borderColor: 'var(--alt-primary)'
        }}
        transition="all 0.2s ease"
        cursor="pointer"
        onClick={handleViewArticle}
      >
        <VStack align="stretch" gap={3}>
          {/* Title with Priority Indicator */}
          <Flex align="center" gap={2}>
            <Text
              fontSize="lg"
              fontWeight="bold"
              color="var(--text-primary)"
              lineHeight="1.4"
              flex={1}
            >
              {feed.title}
            </Text>
            <Text fontSize="sm">
              {feed.metadata?.priority === 'high' ? '🔥' :
               feed.metadata?.priority === 'medium' ? '📈' : '📄'}
            </Text>
          </Flex>

          {/* Description */}
          <Text
            fontSize="sm"
            color="var(--text-secondary)"
            lineHeight="1.5"
            css={{
              display: '-webkit-box',
              WebkitLineClamp: 3,
              WebkitBoxOrient: 'vertical',
              overflow: 'hidden'
            }}
          >
            {feed.description}
          </Text>

                    {/* Tags */}
          <Flex gap={2} flexWrap="wrap">
            {feed.metadata?.tags?.slice(0, 2).map((tag, index) => (
              <Text key={index} fontSize="xs" color="var(--alt-primary)" fontWeight="medium">
                #{tag.toLowerCase()}
              </Text>
            ))}
          </Flex>

          {/* Engagement Stats */}
          <Flex justify="space-between" align="center">
            <Flex gap={4}>
              <Text fontSize="xs" color="var(--text-muted)">
                {feed.metadata?.engagement?.likes || 0} likes
              </Text>
              <Text fontSize="xs" color="var(--text-muted)">
                {feed.metadata?.engagement?.bookmarks || 0} bookmarks
              </Text>
            </Flex>
            <Text fontSize="xs" color="var(--text-muted)">
              {new Date(feed.published).toLocaleDateString()}
            </Text>
          </Flex>

          {/* Action Buttons */}
          <Flex justify="space-between" align="center">
            <Flex gap={2}>
              <button
                aria-label="Mark as Read"
                onClick={handleMarkAsRead}
                style={{
                  fontSize: '12px',
                  color: 'var(--alt-primary)',
                  fontWeight: 'medium',
                  cursor: 'pointer',
                  background: 'none',
                  border: 'none',
                  textDecoration: 'underline'
                }}
              >
                Mark as Read
              </button>
              <button
                aria-label="Toggle favorite"
                title="Favorite"
                onClick={handleToggleFavorite}
                style={{
                  fontSize: '12px',
                  color: 'var(--alt-secondary)',
                  fontWeight: 'medium',
                  cursor: 'pointer',
                  background: 'none',
                  border: 'none',
                  textDecoration: 'underline'
                }}
              >
                Toggle favorite
              </button>
              <button
                aria-label="Toggle bookmark"
                title="Bookmark"
                onClick={handleToggleBookmark}
                style={{
                  fontSize: '12px',
                  color: 'var(--alt-tertiary)',
                  fontWeight: 'medium',
                  cursor: 'pointer',
                  background: 'none',
                  border: 'none',
                  textDecoration: 'underline'
                }}
              >
                Toggle bookmark
              </button>
            </Flex>
            <button
              onClick={handleViewArticle}
              style={{
                fontSize: '12px',
                color: 'var(--alt-primary)',
                fontWeight: 'medium',
                cursor: 'pointer',
                background: 'none',
                border: 'none',
                textDecoration: 'underline'
              }}
            >
              View Article
            </button>
          </Flex>
        </VStack>
      </Box>
    </Box>
  );
});

VirtualizedFeedItem.displayName = 'VirtualizedFeedItem';
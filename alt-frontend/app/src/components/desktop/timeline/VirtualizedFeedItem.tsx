'use client';

import React, { memo } from 'react';
import { Box, VStack, Text, Flex } from '@chakra-ui/react';
import { Feed } from '@/schema/feed';

interface VirtualizedFeedItemProps {
  feed: Feed;
  index: number;
  onMarkAsRead: (feedId: string) => void;
  onToggleFavorite: (feedId: string) => void;
  onViewArticle: (feedId: string) => void;
  style?: React.CSSProperties;
}

export const VirtualizedFeedItem: React.FC<VirtualizedFeedItemProps> = memo(({
  feed,
  index,
  onMarkAsRead,
  onToggleFavorite,
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
          <Text
            fontSize="lg"
            fontWeight="bold"
            color="var(--text-primary)"
            lineHeight="1.4"
          >
            {feed.title}
          </Text>
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
          <Flex justify="space-between" align="center">
            <Text fontSize="xs" color="var(--text-muted)">
              {new Date(feed.published).toLocaleDateString()}
            </Text>
            <Flex gap={2}>
              <Text
                fontSize="xs"
                color="var(--alt-primary)"
                fontWeight="medium"
                cursor="pointer"
                onClick={handleMarkAsRead}
                _hover={{ textDecoration: 'underline' }}
              >
                Mark as Read
              </Text>
              <Text
                fontSize="xs"
                color="var(--alt-secondary)"
                fontWeight="medium"
                cursor="pointer"
                onClick={handleToggleFavorite}
                _hover={{ textDecoration: 'underline' }}
              >
                Favorite
              </Text>
            </Flex>
          </Flex>
        </VStack>
      </Box>
    </Box>
  );
});

VirtualizedFeedItem.displayName = 'VirtualizedFeedItem';
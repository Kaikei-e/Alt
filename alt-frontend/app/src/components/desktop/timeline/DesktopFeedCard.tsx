'use client';

import React, { useState, useCallback, memo, KeyboardEvent } from 'react';
import {
  Box,
  Flex,
  Text,
  Button,
  VStack,
  HStack,
  Badge,
  IconButton,
  Spinner
} from '@chakra-ui/react';
import { 
  Heart, 
  Bookmark, 
  Clock, 
  Eye, 
  ExternalLink
} from 'lucide-react';
import Link from 'next/link';
import { DesktopFeedCardProps } from '@/types/desktop-feed';

const formatTimeAgo = (publishedDate: string): string => {
  const now = new Date();
  const published = new Date(publishedDate);
  const diffMs = now.getTime() - published.getTime();
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
  const diffDays = Math.floor(diffHours / 24);

  if (diffHours < 1) {
    return 'つい今';
  } else if (diffHours < 24) {
    return `${diffHours}時間前`;
  } else if (diffDays < 7) {
    return `${diffDays}日前`;
  } else {
    return `${Math.floor(diffDays / 7)}週間前`;
  }
};

export const DesktopFeedCard = memo(function DesktopFeedCard({
  feed,
  onMarkAsRead,
  onToggleFavorite,
  onToggleBookmark,
  onReadLater,
  onViewArticle
}: DesktopFeedCardProps) {
  const [isLoading, setIsLoading] = useState(false);
  const [isHovered, setIsHovered] = useState(false);

  const handleMarkAsRead = useCallback(async () => {
    if (feed.isRead) return;
    
    setIsLoading(true);
    try {
      await onMarkAsRead(feed.id);
    } finally {
      setIsLoading(false);
    }
  }, [feed.id, feed.isRead, onMarkAsRead]);

  const handleViewArticle = useCallback(() => {
    if (!feed.isRead) {
      handleMarkAsRead();
    }
    onViewArticle(feed.id);
  }, [feed.id, feed.isRead, handleMarkAsRead, onViewArticle]);

  const handleKeyDown = useCallback((event: KeyboardEvent) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      handleViewArticle();
    }
  }, [handleViewArticle]);

  const getPriorityColor = (priority: string) => {
    switch (priority) {
      case 'high': return 'var(--accent-primary)';
      case 'medium': return 'var(--accent-secondary)';
      case 'low': return 'var(--text-muted)';
      default: return 'var(--text-muted)';
    }
  };

  const getPriorityIcon = (priority: string) => {
    switch (priority) {
      case 'high': return '🔥';
      case 'medium': return '📈';
      case 'low': return '📄';
      default: return '📄';
    }
  };

  const getDifficultyColor = (difficulty: string) => {
    switch (difficulty) {
      case 'beginner': return 'var(--alt-success)';
      case 'intermediate': return 'var(--alt-warning)';
      case 'advanced': return 'var(--alt-error)';
      default: return 'var(--text-muted)';
    }
  };

  const formatEngagementStats = () => {
    const { views, comments } = feed.metadata.engagement;
    return [
      views > 0 && `${views} views`,
      comments > 0 && `${comments} comments`
    ].filter(Boolean).join(' • ');
  };

  return (
    <Box
      className="glass"
      p={6}
      borderRadius="var(--radius-xl)"
      border="1px solid var(--surface-border)"
      borderLeftWidth="4px"
      borderLeftColor={getPriorityColor(feed.metadata.priority)}
      cursor="pointer"
      role="article"
      tabIndex={0}
      transition="all var(--transition-smooth) ease"
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      onKeyDown={handleKeyDown}
      aria-label={`Feed: ${feed.title}`}
      data-testid={`desktop-feed-card-${feed.id}`}
      opacity={feed.isRead ? 0.7 : 1}
      _hover={{
        transform: 'translateY(-4px)',
        boxShadow: '0 12px 40px rgba(0, 0, 0, 0.15)',
        borderColor: 'var(--accent-primary)'
      }}
    >
      {/* カードヘッダー */}
      <Flex justify="space-between" align="center" mb={4}>
        <HStack gap={3}>
          <Text fontSize="xl">{feed.metadata.source.icon}</Text>
          <VStack gap={0} align="start">
            <Text
              fontSize="sm"
              fontWeight="semibold"
              color="var(--text-primary)"
            >
              {feed.metadata.source.name}
            </Text>
            <HStack gap={2} fontSize="xs" color="var(--text-secondary)">
              <Text>
                {formatTimeAgo(feed.published)}
              </Text>
              <Text>•</Text>
              <HStack gap={1}>
                <Clock size={12} />
                <Text>{feed.metadata.readingTime} min read</Text>
              </HStack>
            </HStack>
          </VStack>
        </HStack>

        {/* クイックアクション */}
        <HStack 
          gap={1} 
          opacity={isHovered ? 1 : 0}
          transition="opacity var(--transition-speed) ease"
        >
          <IconButton
            aria-label={feed.isFavorited ? 'Remove from favorites' : 'Add to favorites'}
            size="sm"
            variant="ghost"
            color={feed.isFavorited ? 'var(--accent-primary)' : 'var(--text-secondary)'}
            bg={feed.isFavorited ? 'var(--surface-bg)' : 'transparent'}
            onClick={(e) => {
              e.stopPropagation();
              onToggleFavorite(feed.id);
            }}
          >
            <Heart size={16} />
          </IconButton>
          <IconButton
            aria-label={feed.isBookmarked ? 'Remove bookmark' : 'Add bookmark'}
            size="sm"
            variant="ghost"
            color={feed.isBookmarked ? 'var(--accent-secondary)' : 'var(--text-secondary)'}
            bg={feed.isBookmarked ? 'var(--surface-bg)' : 'transparent'}
            onClick={(e) => {
              e.stopPropagation();
              onToggleBookmark(feed.id);
            }}
          >
            <Bookmark size={16} />
          </IconButton>
        </HStack>
      </Flex>

      {/* カードメイン */}
      <VStack gap={4} align="stretch">
        {/* タイトル */}
        <HStack gap={3} align="flex-start">
          <Text fontSize="lg" color={getPriorityColor(feed.metadata.priority)}>
            {getPriorityIcon(feed.metadata.priority)}
          </Text>
          <Link href={feed.link} target="_blank" rel="noopener noreferrer">
            <Text
              fontSize="lg"
              fontWeight="bold"
              color="var(--text-primary)"
              lineHeight="1.4"
              _hover={{ color: 'var(--accent-primary)' }}
              transition="color var(--transition-speed) ease"
            >
              {feed.title}
            </Text>
          </Link>
        </HStack>

        {/* サマリー */}
        {feed.metadata.summary && (
          <Box
            pl={4}
            borderLeft="3px solid var(--surface-border)"
            bg="var(--surface-bg)"
            p={3}
            borderRadius="var(--radius-md)"
          >
            <Text
              fontSize="sm"
              color="var(--text-secondary)"
              fontStyle="italic"
              lineHeight="1.6"
            >
              💬 &quot;{feed.metadata.summary}&quot;
            </Text>
          </Box>
        )}

        {/* エンゲージメント統計 */}
        <HStack gap={4} fontSize="sm" color="var(--text-secondary)">
          <HStack gap={1}>
            <Eye size={14} />
            <Text>{formatEngagementStats()}</Text>
          </HStack>
          {feed.metadata.relatedCount > 0 && (
            <HStack gap={1}>
              <Text>📈</Text>
              <Text>{feed.metadata.relatedCount} related</Text>
            </HStack>
          )}
          <HStack gap={1}>
            <Text>🎯</Text>
            <Badge
              size="sm"
              bg="var(--surface-bg)"
              color={getDifficultyColor(feed.metadata.difficulty)}
              border="1px solid var(--surface-border)"
              textTransform="capitalize"
            >
              {feed.metadata.difficulty}
            </Badge>
          </HStack>
        </HStack>

        {/* タグ */}
        {feed.metadata.tags.length > 0 && (
          <HStack gap={2} flexWrap="wrap">
            <Text fontSize="sm" color="var(--text-secondary)">🏷️</Text>
            {feed.metadata.tags.slice(0, 4).map((tag, index) => (
              <Badge
                key={index}
                bg="var(--accent-primary)"
                color="white"
                fontSize="xs"
                px={2}
                py={1}
                borderRadius="var(--radius-full)"
                _hover={{
                  transform: 'translateY(-1px)',
                  boxShadow: '0 4px 8px rgba(0, 0, 0, 0.1)'
                }}
                transition="all var(--transition-speed) ease"
              >
                #{tag}
              </Badge>
            ))}
            {feed.metadata.tags.length > 4 && (
              <Text fontSize="xs" color="var(--text-muted)">
                +{feed.metadata.tags.length - 4} more
              </Text>
            )}
          </HStack>
        )}
      </VStack>

      {/* カードフッター */}
      <Flex 
        justify="space-between" 
        align="center" 
        mt={6}
        pt={4}
        borderTop="1px solid var(--surface-border)"
      >
        <HStack gap={3}>
          <Button
            size="sm"
            bg={feed.isRead ? 'var(--text-muted)' : 'var(--accent-primary)'}
            color="white"
            fontWeight="bold"
            borderRadius="var(--radius-full)"
            disabled={feed.isRead || isLoading}
            onClick={(e) => {
              e.stopPropagation();
              handleMarkAsRead();
            }}
            _hover={{
              bg: feed.isRead ? 'var(--text-muted)' : 'var(--accent-secondary)',
              transform: 'scale(1.05)'
            }}
            _disabled={{
              opacity: 0.6,
              cursor: 'not-allowed'
            }}
          >
            {isLoading ? (
              <Spinner size="xs" />
            ) : feed.isRead ? (
              'Read'
            ) : (
              'Mark as Read'
            )}
          </Button>

          <Button
            size="sm"
            variant="outline"
            borderColor="var(--surface-border)"
            color="var(--text-secondary)"
            borderRadius="var(--radius-full)"
            onClick={(e) => {
              e.stopPropagation();
              onReadLater(feed.id);
            }}
            _hover={{
              bg: 'var(--surface-hover)',
              borderColor: 'var(--accent-secondary)'
            }}
          >
            Read Later
          </Button>
        </HStack>

        <Button
          size="sm"
          bg="var(--accent-gradient)"
          color="white"
          fontWeight="bold"
          borderRadius="var(--radius-full)"
          onClick={(e) => {
            e.stopPropagation();
            handleViewArticle();
          }}
          _hover={{
            transform: 'scale(1.05)',
            filter: 'brightness(1.1)'
          }}
        >
          View Article <ExternalLink size={14} style={{ marginLeft: '4px' }} />
        </Button>
      </Flex>

      {/* 読書進捗表示（既読の場合） */}
      {feed.isRead && feed.readingProgress && (
        <Box mt={4}>
          <Box
            bg="var(--surface-border)"
            borderRadius="var(--radius-full)"
            h="4px"
            position="relative"
            overflow="hidden"
          >
            <Box
              bg="var(--accent-primary)"
              h="100%"
              borderRadius="var(--radius-full)"
              width={`${feed.readingProgress}%`}
              transition="width 0.3s ease"
            />
          </Box>
          <Text fontSize="xs" color="var(--text-muted)" mt={1}>
            Reading progress: {feed.readingProgress}%
          </Text>
        </Box>
      )}
    </Box>
  );
});
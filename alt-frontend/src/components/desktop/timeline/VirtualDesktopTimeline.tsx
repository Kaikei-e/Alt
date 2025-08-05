"use client";

import React, { useRef, useCallback, useMemo } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Box, Text, VStack } from "@chakra-ui/react";
import { Feed } from "@/schema/feed";
import { DesktopFeed } from "@/types/desktop-feed";
import { DesktopFeedCard } from "./DesktopFeedCard";
import { useVirtualizationMetrics } from "@/hooks/useVirtualizationMetrics";

interface VirtualDesktopTimelineProps {
  feeds: Feed[];
  readFeeds: Set<string>;
  onMarkAsRead: (feedId: string) => void;
  onToggleFavorite: (feedId: string) => void;
  onToggleBookmark: (feedId: string) => void;
  onReadLater: (feedId: string) => void;
  onViewArticle: (feedId: string) => void;
  containerHeight: number;
  enableDynamicSizing?: boolean;
  overscan?: number;
}

// Desktop-specific size estimation
const estimateDesktopItemSize = (feed: Feed): number => {
  const baseHeight = 280; // Desktop card base height
  const titleHeight = Math.ceil(feed.title.length / 60) * 24; // 60 chars/line
  const descriptionHeight = Math.ceil(feed.description.length / 80) * 20; // 80 chars/line
  const metadataHeight = 60; // Metadata section
  const actionHeight = 50; // Action buttons

  return (
    baseHeight + titleHeight + descriptionHeight + metadataHeight + actionHeight
  );
};

// Transform Feed to DesktopFeed
const transformToDesktopFeed = (feed: Feed): DesktopFeed => {
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
  containerHeight,
  enableDynamicSizing = false,
  overscan = 2,
}) => {
  const parentRef = useRef<HTMLDivElement>(null);

  // Performance metrics
  useVirtualizationMetrics(true, feeds.length);

  // Transform feeds to desktop format
  const desktopFeeds = useMemo(() => {
    return feeds?.map(transformToDesktopFeed) || [];
  }, [feeds]);

  // Filter out read feeds
  const visibleFeeds = useMemo(
    () => desktopFeeds.filter((feed) => !readFeeds.has(feed.id)),
    [desktopFeeds, readFeeds],
  );

  // Dynamic size estimation
  const estimateSize = useCallback(
    (index: number) => {
      if (enableDynamicSizing && visibleFeeds[index]) {
        return estimateDesktopItemSize(visibleFeeds[index]);
      }
      return 320; // Default desktop card height
    },
    [visibleFeeds, enableDynamicSizing],
  );

  // Measurement function for dynamic sizing
  const measureElement = useCallback(
    (element: Element) => {
      if (enableDynamicSizing && element instanceof HTMLElement) {
        // Actual measurement implementation if needed
        // Currently using estimated values
      }
      return 0;
    },
    [enableDynamicSizing],
  );

  // Create virtualizer with desktop-specific settings
  const virtualizer = useVirtualizer({
    count: visibleFeeds.length,
    getScrollElement: () => parentRef.current,
    estimateSize,
    overscan,
    measureElement: enableDynamicSizing ? measureElement : undefined,
  });

  // Handle actions
  const handleMarkAsRead = useCallback(
    (feedId: string) => {
      onMarkAsRead(feedId);
    },
    [onMarkAsRead],
  );

  const handleToggleFavorite = useCallback(
    (feedId: string) => {
      onToggleFavorite(feedId);
    },
    [onToggleFavorite],
  );

  const handleToggleBookmark = useCallback(
    (feedId: string) => {
      onToggleBookmark(feedId);
    },
    [onToggleBookmark],
  );

  const handleReadLater = useCallback(
    (feedId: string) => {
      onReadLater(feedId);
    },
    [onReadLater],
  );

  const handleViewArticle = useCallback(
    (feedId: string) => {
      onViewArticle(feedId);
    },
    [onViewArticle],
  );

  // Empty state handling
  if (visibleFeeds.length === 0) {
    return (
      <Box
        height={`${containerHeight}px`}
        display="flex"
        alignItems="center"
        justifyContent="center"
        data-testid="virtual-desktop-empty-state"
      >
        <VStack gap={4}>
          <Text fontSize="2xl">📰</Text>
          <Text color="var(--text-primary)" fontSize="lg">
            No feeds available
          </Text>
          <Text color="var(--text-secondary)" fontSize="sm">
            Your feed will appear here once you subscribe to sources
          </Text>
        </VStack>
      </Box>
    );
  }

  return (
    <Box
      ref={parentRef}
      height={`${containerHeight}px`}
      overflowY="auto"
      overflowX="hidden"
      data-testid="virtual-desktop-timeline"
      css={{
        scrollBehavior: enableDynamicSizing ? "auto" : "smooth",
        "&::-webkit-scrollbar": {
          width: "8px",
        },
        "&::-webkit-scrollbar-track": {
          background: "var(--surface-secondary)",
          borderRadius: "4px",
        },
        "&::-webkit-scrollbar-thumb": {
          background: "var(--accent-primary)",
          borderRadius: "4px",
          opacity: 0.7,
        },
        "&::-webkit-scrollbar-thumb:hover": {
          opacity: 1,
        },
      }}
    >
      <Box
        height={`${virtualizer.getTotalSize()}px`}
        width="100%"
        position="relative"
        maxW="1200px"
        mx="auto"
        px={6}
      >
        {virtualizer.getVirtualItems().map((virtualItem) => {
          const feed = visibleFeeds[virtualItem.index];

          return (
            <Box
              key={virtualItem.key}
              data-testid={`virtual-desktop-item-${virtualItem.index}`}
              data-index={virtualItem.index}
              ref={enableDynamicSizing ? virtualizer.measureElement : undefined}
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

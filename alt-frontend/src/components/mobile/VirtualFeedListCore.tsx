"use client";

import { useVirtualizer } from "@tanstack/react-virtual";
import React, { useRef, useCallback, useEffect } from "react";
import { Box, Text } from "@chakra-ui/react";
import { Feed } from "@/schema/feed";
import FeedCard from "./FeedCard";

interface VirtualFeedListCoreProps {
  feeds: Feed[];
  readFeeds: Set<string>;
  onMarkAsRead: (feedLink: string) => void;
  estimatedItemHeight: number;
  containerHeight: number;
  overscan?: number;
}

export const VirtualFeedListCore: React.FC<VirtualFeedListCoreProps> = ({
  feeds,
  readFeeds,
  onMarkAsRead,
  estimatedItemHeight,
  containerHeight,
  overscan = 5,
}) => {
  const parentRef = useRef<HTMLDivElement>(null);

  // 仮想化設定
  const virtualizer = useVirtualizer({
    count: feeds.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => estimatedItemHeight,
    overscan,
    // 固定サイズモード（measureElement を使用しない）
    measureElement: undefined,
  });

  // スクロール位置の保持
  useEffect(() => {
    if (parentRef.current) {
      parentRef.current.scrollTop = 0;
    }
  }, [feeds.length]);

  const handleMarkAsRead = useCallback(
    (feedLink: string) => {
      onMarkAsRead(feedLink);
    },
    [onMarkAsRead],
  );

  if (feeds.length === 0) {
    return (
      <Box
        height={`${containerHeight}px`}
        display="flex"
        alignItems="center"
        justifyContent="center"
        data-testid="virtual-empty-state"
      >
        <Text color="var(--text-secondary)">No feeds available</Text>
      </Box>
    );
  }

  return (
    <Box
      ref={parentRef}
      height={`${containerHeight}px`}
      overflow="auto"
      data-testid="virtual-scroll-container"
      css={{
        scrollBehavior: "smooth",
        "&::-webkit-scrollbar": {
          width: "6px",
        },
        "&::-webkit-scrollbar-track": {
          background: "var(--surface-secondary)",
          borderRadius: "3px",
        },
        "&::-webkit-scrollbar-thumb": {
          background: "var(--accent-primary)",
          borderRadius: "3px",
          opacity: 0.7,
        },
      }}
    >
      <Box
        height={`${virtualizer.getTotalSize()}px`}
        width="100%"
        position="relative"
        data-testid="virtual-content-container"
      >
        {virtualizer.getVirtualItems().map((virtualItem) => {
          const feed = feeds[virtualItem.index];

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
              px={2}
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

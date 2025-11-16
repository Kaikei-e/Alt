"use client";

import { Box, Text } from "@chakra-ui/react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type React from "react";
import { useCallback, useEffect, useRef } from "react";
import type { Feed } from "@/schema/feed";
import { SizeMeasurementManager } from "@/utils/sizeMeasurement";
import FeedCard from "./FeedCard";

interface DynamicVirtualFeedListProps {
  feeds: Feed[];
  readFeeds: Set<string>;
  onMarkAsRead: (feedLink: string) => void;
  containerHeight: number;
  overscan?: number;
  onMeasurementError?: (error: Error) => void;
}

export const DynamicVirtualFeedList: React.FC<DynamicVirtualFeedListProps> = ({
  feeds,
  readFeeds,
  onMarkAsRead,
  containerHeight,
  overscan = 5,
  onMeasurementError,
}) => {
  const parentRef = useRef<HTMLDivElement>(null);
  const measurementManager = useRef(
    new SizeMeasurementManager(onMeasurementError),
  );

  // 動的サイズ推定関数
  const estimateSize = useCallback(
    (index: number) => {
      const feed = feeds[index];
      if (!feed) return 200;

      const contentLength = feed.title.length + feed.description.length;
      return measurementManager.current.getEstimatedSize(contentLength);
    },
    [feeds],
  );

  // 実際のサイズ測定関数
  const measureElement = useCallback(() => {
    // For dynamic sizing, we don't need to return a specific size
    // The measurement is handled by the measurement manager
    return 0;
  }, []);

  // 仮想化設定
  const virtualizer = useVirtualizer({
    count: feeds.length,
    getScrollElement: () => parentRef.current,
    estimateSize: estimateSize,
    overscan,
    measureElement: measureElement,
    // スムーズスクロール無効化（Dynamic Sizingでは非対応）
    scrollToFn: (offset) => {
      const element = parentRef.current;
      if (element) {
        element.scrollTop = offset;
      }
    },
  });

  // フィードデータが変更された時にキャッシュをクリア
  useEffect(() => {
    measurementManager.current.clearCache();
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
        data-testid="dynamic-virtual-empty-state"
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
        // スムーズスクロールを無効化（Dynamic Sizingでは問題が発生する可能性）
        scrollBehavior: "auto",
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
        data-testid="dynamic-virtual-content-container"
      >
        {virtualizer.getVirtualItems().map((virtualItem) => {
          const feed = feeds[virtualItem.index];

          return (
            <Box
              key={virtualItem.key}
              data-testid={`virtual-feed-item-${virtualItem.index}`}
              data-index={virtualItem.index}
              ref={virtualizer.measureElement}
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

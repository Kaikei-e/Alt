"use client";

import { Box, VStack } from "@chakra-ui/react";
import { useMemo, useState, useEffect, useRef } from "react";
import type { SearchFeedItem } from "@/schema/search";
import { SearchResultItem } from "./SearchResults";

interface SearchResultsVirtualListProps {
  results: SearchFeedItem[];
  itemHeight?: number;
  overscan?: number;
}

const DEFAULT_ITEM_HEIGHT = 200;
const DEFAULT_OVERSCAN = 3;

export function SearchResultsVirtualList({
  results,
  itemHeight = DEFAULT_ITEM_HEIGHT,
  overscan = DEFAULT_OVERSCAN,
}: SearchResultsVirtualListProps) {
  const [containerHeight, setContainerHeight] = useState(0);
  const [scrollTop, setScrollTop] = useState(0);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const updateHeight = () => {
      setContainerHeight(container.clientHeight);
    };

    updateHeight();
    window.addEventListener("resize", updateHeight);
    return () => window.removeEventListener("resize", updateHeight);
  }, []);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const handleScroll = () => {
      setScrollTop(container.scrollTop);
    };

    container.addEventListener("scroll", handleScroll, { passive: true });
    return () => container.removeEventListener("scroll", handleScroll);
  }, []);

  const { startIndex, endIndex, totalHeight, offsetY } = useMemo(() => {
    const totalItems = results.length;
    const totalHeight = totalItems * itemHeight;

    if (containerHeight === 0) {
      return {
        startIndex: 0,
        endIndex: Math.min(overscan * 2, totalItems),
        totalHeight,
        offsetY: 0,
      };
    }

    const startIndex = Math.max(
      0,
      Math.floor(scrollTop / itemHeight) - overscan,
    );
    const visibleCount = Math.ceil(containerHeight / itemHeight);
    const endIndex = Math.min(
      totalItems,
      startIndex + visibleCount + overscan * 2,
    );

    return {
      startIndex,
      endIndex,
      totalHeight,
      offsetY: startIndex * itemHeight,
    };
  }, [results.length, itemHeight, containerHeight, scrollTop, overscan]);

  const visibleItems = useMemo(() => {
    return results.slice(startIndex, endIndex);
  }, [results, startIndex, endIndex]);

  return (
    <Box
      ref={containerRef}
      height="100%"
      overflowY="auto"
      overflowX="hidden"
      style={{ overscrollBehavior: "contain" }}
    >
      <Box position="relative" height={`${totalHeight}px`} width="100%">
        <VStack
          align="stretch"
          gap={4}
          position="absolute"
          top={`${offsetY}px`}
          left={0}
          right={0}
        >
          {visibleItems.map((result, index) => (
            <Box
              key={result.link || `result-${startIndex + index}`}
              minHeight={`${itemHeight}px`}
            >
              <SearchResultItem result={result} />
            </Box>
          ))}
        </VStack>
      </Box>
    </Box>
  );
}

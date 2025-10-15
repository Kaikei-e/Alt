"use client";

import { Box, Flex } from "@chakra-ui/react";
import SkeletonFeedCard from "@/components/mobile/SkeletonFeedCard";
import EmptyFeedState from "@/components/mobile/EmptyFeedState";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import SwipeFeedCard from "@/components/mobile/feeds/swipe/SwipeFeedCard";
import { useSwipeFeedController } from "@/components/mobile/feeds/swipe/useSwipeFeedController";
import ErrorState from "@/app/mobile/feeds/_components/ErrorState";

const SwipeFeedSkeleton = () => (
  <Box minH="100vh" position="relative">
    <Box
      p={5}
      maxW="container.sm"
      mx="auto"
      height="100vh"
      data-testid="swipe-skeleton-container"
    >
      <Flex direction="column" gap={4}>
        {Array.from({ length: 5 }).map((_, index) => (
          <SkeletonFeedCard key={`swipe-skeleton-${index}`} />
        ))}
      </Flex>
    </Box>
    <FloatingMenu />
  </Box>
);

const LiveRegion = ({ message }: { message: string }) => (
  <Box
    aria-live="polite"
    aria-atomic="true"
    position="absolute"
    left="-10000px"
    width="1px"
    height="1px"
    overflow="hidden"
  >
    {message}
  </Box>
);

const SwipeFeedScreen = () => {
  const {
    feeds,
    activeFeed,
    activeIndex,
    hasMore,
    isInitialLoading,
    isValidating,
    error,
    liveRegionMessage,
    statusMessage,
    dismissActiveFeed,
    retry,
  } = useSwipeFeedController();

  if (isInitialLoading) {
    return <SwipeFeedSkeleton />;
  }

  if (error) {
    return <ErrorState error={error} onRetry={retry} isLoading={isValidating} />;
  }

  const isOutOfFeeds = !activeFeed || activeIndex >= feeds.length;

  if (isOutOfFeeds) {
    if (hasMore || isValidating) {
      return <SwipeFeedSkeleton />;
    }

    return (
      <Box minH="100vh" position="relative">
        <EmptyFeedState />
        <FloatingMenu />
      </Box>
    );
  }

  return (
    <Box minH="100vh" position="relative">
      <LiveRegion message={liveRegionMessage} />

      <Flex
        direction="column"
        align="center"
        justify="center"
        h="100dvh"
        px={4}
        style={{
          overscrollBehavior: "contain",
          touchAction: "pan-y",
        }}
      >
        <SwipeFeedCard
          key={activeFeed.id}
          feed={activeFeed}
          statusMessage={statusMessage}
          onDismiss={dismissActiveFeed}
        />
      </Flex>

      <FloatingMenu />
    </Box>
  );
};

export default SwipeFeedScreen;

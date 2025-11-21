"use client";

import { keyframes } from "@emotion/react";
import { Box, Flex, HStack, Icon, Text, VStack } from "@chakra-ui/react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useEffect, useState } from "react";
import ErrorState from "@/app/mobile/feeds/_components/ErrorState";
import EmptyFeedState from "@/components/mobile/EmptyFeedState";
import dynamic from "next/dynamic";
import { useSwipeFeedController } from "@/components/mobile/feeds/swipe/useSwipeFeedController";
import SkeletonFeedCard from "@/components/mobile/SkeletonFeedCard";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";

const usePrefersReducedMotion = () => {
  const [prefersReducedMotion, setPrefersReducedMotion] = useState(() => {
    if (
      typeof window === "undefined" ||
      typeof window.matchMedia !== "function"
    ) {
      return false;
    }
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  });

  useEffect(() => {
    if (
      typeof window === "undefined" ||
      typeof window.matchMedia !== "function"
    ) {
      return;
    }

    const mediaQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
    const updatePreference = (event: MediaQueryListEvent | MediaQueryList) => {
      setPrefersReducedMotion(event.matches);
    };

    updatePreference(mediaQuery);

    mediaQuery.addEventListener("change", updatePreference);

    return () => {
      mediaQuery.removeEventListener("change", updatePreference);
    };
  }, []);

  return prefersReducedMotion;
};

const arrowDrift = keyframes`
  0% { transform: translateX(-4px); opacity: 0.4; }
  50% { transform: translateX(4px); opacity: 1; }
  100% { transform: translateX(-4px); opacity: 0.4; }
`;

const dotPulse = keyframes`
  0%, 100% { opacity: 0.3; transform: scale(0.9); }
  50% { opacity: 0.9; transform: scale(1); }
`;

const loadingBar = keyframes`
  0% { transform: translateX(-60%); }
  50% { transform: translateX(-10%); }
  100% { transform: translateX(120%); }
`;

const SwipeSkeletonHint = ({
  prefersReducedMotion,
}: {
  prefersReducedMotion: boolean;
}) => (
  <VStack
    gap={3}
    align="center"
    data-testid="swipe-skeleton-hint"
    data-reduced-motion={prefersReducedMotion ? "true" : "false"}
  >
    <Text fontSize="sm" color="var(--alt-text-secondary)">
      スワイプで次の記事へ進めます
    </Text>
    <HStack gap={3} color="var(--alt-primary)">
      <Icon
        as={ChevronLeft}
        boxSize={6}
        opacity={0.9}
        style={
          prefersReducedMotion
            ? undefined
            : {
              animation: `${arrowDrift} 1.6s ease-in-out infinite`,
            }
        }
      />
      {Array.from({ length: 3 }).map((_, index) => (
        <Box
          key={`swipe-dot-${index}`}
          w={2}
          h={2}
          borderRadius="full"
          bg="var(--alt-primary)"
          opacity={0.6}
          style={
            prefersReducedMotion
              ? undefined
              : {
                animation: `${dotPulse} 1.8s ${(index + 1) * 0.12}s ease-in-out infinite`,
              }
          }
        />
      ))}
      <Icon
        as={ChevronRight}
        boxSize={6}
        opacity={0.9}
        style={
          prefersReducedMotion
            ? undefined
            : {
              animation: `${arrowDrift} 1.6s ease-in-out infinite reverse`,
            }
        }
      />
    </HStack>
  </VStack>
);

const SwipeFeedSkeleton = ({
  prefersReducedMotion,
}: {
  prefersReducedMotion: boolean;
}) => (
  <Box minH="100dvh" position="relative">
    <Box
      p={5}
      maxW="container.sm"
      mx="auto"
      height="100dvh"
      data-testid="swipe-skeleton-container"
      display="flex"
      alignItems="center"
      justifyContent="center"
    >
      <VStack gap={8} w="100%">
        <SkeletonFeedCard variant="swipe" reduceMotion={prefersReducedMotion} />
        <SwipeSkeletonHint prefersReducedMotion={prefersReducedMotion} />
      </VStack>
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

const SwipeLoadingOverlay = ({
  isVisible,
  reduceMotion,
}: {
  isVisible: boolean;
  reduceMotion: boolean;
}) => {
  if (!isVisible) {
    return null;
  }

  return (
    <Box
      data-testid="swipe-progress-indicator"
      position="absolute"
      left={0}
      right={0}
      bottom={0}
      px={4}
      pb="calc(1rem + env(safe-area-inset-bottom, 0px))"
      pointerEvents="none"
      zIndex={10}
    >
      <Box
        borderRadius="xl"
        border="1px solid var(--alt-glass-border)"
        bg="rgba(10, 10, 20, 0.85)"
        p={4}
        maxW="26rem"
        mx="auto"
        boxShadow="0 8px 30px rgba(0,0,0,0.35)"
      >
        <Text
          fontSize="xs"
          color="var(--alt-text-secondary)"
          textAlign="center"
        >
          新しい記事を読み込んでいます
        </Text>
        <Box
          mt={3}
          height="4px"
          borderRadius="full"
          bg="rgba(255,255,255,0.12)"
          overflow="hidden"
          aria-hidden="true"
        >
          <Box
            height="100%"
            width="45%"
            borderRadius="full"
            bg="var(--alt-primary)"
            opacity={0.85}
            style={
              reduceMotion
                ? { transform: "translateX(0)" }
                : { animation: `${loadingBar} 1.4s ease-in-out infinite` }
            }
          />
        </Box>
        <Box
          as="span"
          aria-live="polite"
          position="absolute"
          width="1px"
          height="1px"
          padding={0}
          margin="-1px"
          overflow="hidden"
          clip="rect(0, 0, 0, 0)"
          whiteSpace="nowrap"
          border={0}
        >
          新しい記事を読み込んでいます
        </Box>
      </Box>
    </Box>
  );
};

const SwipeFeedCard = dynamic(
  () => import("@/components/mobile/feeds/swipe/SwipeFeedCard"),
  {
    loading: () => (
      <Box
        w="100%"
        maxW="30rem"
        h="95dvh"
        bg="var(--alt-glass)"
        borderRadius="1rem"
        border="2px solid var(--alt-glass-border)"
        display="flex"
        alignItems="center"
        justifyContent="center"
      >
        <SkeletonFeedCard variant="swipe" />
      </Box>
    ),
    ssr: false,
  },
);

const SwipeFeedScreen = () => {
  const {
    feeds,
    activeFeed,
    hasMore,
    isInitialLoading,
    isValidating,
    error,
    liveRegionMessage,
    statusMessage,
    dismissActiveFeed,
    retry,
    getCachedContent,
  } = useSwipeFeedController();

  const prefersReducedMotion = usePrefersReducedMotion();
  const shouldShowOverlay = Boolean(activeFeed) && isValidating;

  if (error) {
    return (
      <ErrorState error={error} onRetry={retry} isLoading={isValidating} />
    );
  }

  if (isInitialLoading) {
    return <SwipeFeedSkeleton prefersReducedMotion={prefersReducedMotion} />;
  }

  if (activeFeed) {
    return (
      <Box minH="100dvh" position="relative">
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
            getCachedContent={getCachedContent}
            isBusy={shouldShowOverlay}
          />
        </Flex>

        <SwipeLoadingOverlay
          isVisible={shouldShowOverlay}
          reduceMotion={prefersReducedMotion}
        />

        <FloatingMenu />
      </Box>
    );
  }

  if (hasMore) {
    return <SwipeFeedSkeleton prefersReducedMotion={prefersReducedMotion} />;
  }

  return (
    <Box minH="100dvh" position="relative">
      <EmptyFeedState />
      <FloatingMenu />
    </Box>
  );
};

export default SwipeFeedScreen;

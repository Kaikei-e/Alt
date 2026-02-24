"use client";

import { keyframes } from "@emotion/react";
import { Box, Flex, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import ErrorState from "@/app/mobile/feeds/_components/ErrorState";
import EmptyFeedState from "@/components/mobile/EmptyFeedState";
import { useSwipeFeedController } from "@/components/mobile/feeds/swipe/useSwipeFeedController";
import SwipeFeedSkeleton from "@/components/mobile/feeds/swipe/SwipeFeedSkeleton";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import type { RenderFeed } from "@/schema/feed";
import type { SafeHtmlString } from "@/lib/server/sanitize-html";

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

const loadingBar = keyframes`
  0% { transform: translateX(-60%); }
  50% { transform: translateX(-10%); }
  100% { transform: translateX(120%); }
`;

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

import SwipeFeedCard from "@/components/mobile/feeds/swipe/SwipeFeedCard";

interface SwipeFeedScreenProps {
  initialFeeds?: RenderFeed[];
  initialNextCursor?: string;
  initialArticleContent?: SafeHtmlString | null;
}

const SwipeFeedScreen = ({
  initialFeeds,
  initialNextCursor,
  initialArticleContent,
}: SwipeFeedScreenProps) => {
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
  } = useSwipeFeedController(initialFeeds, initialNextCursor);

  const prefersReducedMotion = usePrefersReducedMotion();
  const shouldShowOverlay = Boolean(activeFeed) && isValidating;

  // Delay rendering of chrome (FloatingMenu, LoadingOverlay) until after hydration
  const [showChrome, setShowChrome] = useState(false);

  useEffect(() => {
    // Show chrome after activeFeed is determined and hydration completes
    if (activeFeed) {
      // Use requestIdleCallback to defer chrome rendering
      if ("requestIdleCallback" in window) {
        const idleCallbackId = window.requestIdleCallback(
          () => {
            setShowChrome(true);
          },
          { timeout: 1000 },
        );
        return () => {
          window.cancelIdleCallback(idleCallbackId);
        };
      } else {
        const timeoutId = setTimeout(() => {
          setShowChrome(true);
        }, 100);
        return () => clearTimeout(timeoutId);
      }
    }
  }, [activeFeed]);

  if (error) {
    return (
      <ErrorState error={error} onRetry={retry} isLoading={isValidating} />
    );
  }

  // Show loading skeleton only during true initial load (no feeds, no initialFeeds)
  if (isInitialLoading) {
    return <SwipeFeedSkeleton prefersReducedMotion={prefersReducedMotion} />;
  }

  // If we have an active feed, show it immediately
  // This handles both SSR initialFeeds and client-fetched feeds
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
          }}
        >
          <SwipeFeedCard
            key={activeFeed.id}
            feed={activeFeed}
            statusMessage={statusMessage}
            onDismiss={dismissActiveFeed}
            getCachedContent={getCachedContent}
            isBusy={shouldShowOverlay}
            initialArticleContent={
              activeFeed.id === initialFeeds?.[0]?.id
                ? (initialArticleContent ?? undefined)
                : undefined
            }
          />
        </Flex>

        {/* Delay chrome rendering until after LCP */}
        {showChrome && (
          <>
            <SwipeLoadingOverlay
              isVisible={shouldShowOverlay}
              reduceMotion={prefersReducedMotion}
            />
            <FloatingMenu />
          </>
        )}
      </Box>
    );
  }

  // If we have more feeds available but none are currently shown,
  // show loading skeleton while fetching
  if (hasMore && isValidating) {
    return <SwipeFeedSkeleton prefersReducedMotion={prefersReducedMotion} />;
  }

  // No feeds available and no more to fetch
  return (
    <Box minH="100dvh" position="relative">
      <EmptyFeedState />
      <FloatingMenu />
    </Box>
  );
};

export default SwipeFeedScreen;

"use client";

import { Box, Button, Flex, Text } from "@chakra-ui/react";
import { Infinity as InfinityIcon } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  startTransition,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import EmptyFeedState from "@/components/mobile/EmptyFeedState";
import SkeletonFeedCard from "@/components/mobile/SkeletonFeedCard";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import VirtualFeedList from "@/components/mobile/VirtualFeedList";
import { useAuth } from "@/contexts/auth-context";
import { useCursorPagination } from "@/hooks/useCursorPagination";
import { feedApi } from "@/lib/api";
import { useInfiniteScroll } from "@/lib/utils/infiniteScroll";
import type { Feed } from "@/schema/feed";
import ErrorState from "./_components/ErrorState";

const PAGE_SIZE = 20;

// URL正規化関数（TODO.mdの指示に基づく）
const canonicalize = (url: string) => {
  try {
    const u = new URL(url);
    u.hash = "";
    [
      "utm_source",
      "utm_medium",
      "utm_campaign",
      "utm_term",
      "utm_content",
    ].forEach((k) => u.searchParams.delete(k));
    if (u.pathname !== "/" && u.pathname.endsWith("/"))
      u.pathname = u.pathname.slice(0, -1);
    return u.toString();
  } catch {
    return url;
  }
};

export default function FeedsPage() {
  const _router = useRouter();
  const { isAuthenticated, isLoading: authLoading, user } = useAuth();
  const [readFeeds, setReadFeeds] = useState<Set<string>>(new Set());
  const [liveRegionMessage, setLiveRegionMessage] = useState<string>("");
  const [isRetrying, setIsRetrying] = useState(false);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  // Additional debug logging for auth state
  useEffect(() => {
    if (process.env.NODE_ENV === "development") {
      // Debug logging can be added here if needed
    }
  }, [isAuthenticated, authLoading, user]);

  // Ensure we start at the top of the list on first render (some mobile browsers
  // restore scroll position across navigations, which can leave us mid-way in
  // a virtualized list with no items rendered yet).
  useEffect(() => {
    const el = scrollContainerRef.current;
    if (el) {
      el.scrollTop = 0;
    }
  }, []);

  // Use cursor-based pagination hook
  const {
    data: feeds,
    hasMore,
    isLoading,
    error,
    isInitialLoading,
    loadMore,
    refresh,
  } = useCursorPagination<Feed>(feedApi.getFeedsWithCursor, {
    limit: PAGE_SIZE,
    autoLoad: true,
  });

  // Memoize visible feeds to prevent unnecessary recalculations
  // Canonicalize feed.link to match the canonicalized keys in readFeeds Set
  const visibleFeeds = useMemo(
    () =>
      feeds?.filter((feed) => !readFeeds.has(canonicalize(feed.link))) || [],
    [feeds, readFeeds],
  );

  // Handle marking feed as read with optimistic update + API call (TODO.mdの指示に基づく)
  const handleMarkAsRead = useCallback(async (rawLink: string) => {
    const link = canonicalize(rawLink);

    // 楽観更新（即時にUIから消す）
    startTransition(() => {
      setReadFeeds((prev) => new Set(prev).add(link));
    });
    setLiveRegionMessage("Feed marked as read");
    setTimeout(() => setLiveRegionMessage(""), 1000);

    // サーバ更新（失敗時はロールバック）
    try {
      await feedApi.updateFeedReadStatus(link);
    } catch (e) {
      startTransition(() => {
        setReadFeeds((prev) => {
          const next = new Set(prev);
          next.delete(link);
          return next;
        });
      });
      // エラートーストの表示（TODO: 必要に応じてトースト表示を追加）
      console.error("Failed to mark feed as read:", e);
    }
  }, []);

  // Retry functionality with exponential backoff
  const retryFetch = useCallback(async () => {
    setIsRetrying(true);

    try {
      await refresh();
    } catch (err) {
      console.error("Retry failed:", err);
      throw err; // Re-throw to let ErrorState handle retry logic
    } finally {
      setIsRetrying(false);
    }
  }, [refresh]);

  // Use infinite scroll hook with proper callback
  const handleLoadMore = useCallback(() => {
    if (hasMore && !isLoading) {
      loadMore();
    }
  }, [hasMore, isLoading, loadMore]);

  useInfiniteScroll(handleLoadMore, sentinelRef, feeds?.length || 0, {
    throttleDelay: 200, // Increased throttle for better stability
    rootMargin: "100px 0px", // Trigger loading a bit earlier
    threshold: 0.1,
  });

  // Show auth loading state
  if (authLoading) {
    return (
      <Box minH="100dvh" position="relative">
        <Box
          p={5}
          maxW="container.sm"
          mx="auto"
          height="100dvh"
          data-testid="feeds-auth-loading"
        >
          <Flex direction="column" gap={4}>
            {Array.from({ length: 5 }).map((_, index) => (
              <SkeletonFeedCard key={`skeleton-${index}`} />
            ))}
          </Flex>
        </Box>
        <FloatingMenu />
      </Box>
    );
  }

  // 🚨 REMOVED: Client-side authentication check
  // Middleware (middleware.ts) already handles session validation and redirects
  // to /public/landing if the user is not authenticated. If this component
  // renders, we know the user has a valid session.
  //
  // The previous client-side check was causing infinite redirect loops because:
  // 1. Kratos session cookies may be HttpOnly (not readable by client JS)
  // 2. Client-side useAuth.isAuthenticated was false even with valid session
  // 3. This caused repeated renders of the "login required" UI
  // 4. Next.js RSC repeatedly fetched /auth/login (_rsc parameter)
  // 5. Loop: /mobile/feeds → /auth/login → /ory/... → /mobile/feeds
  //
  // Trust the middleware. If we're here, the user is authenticated.

  // Show skeleton loading state for immediate visual feedback
  if (isInitialLoading) {
    return (
      <Box minH="100dvh" position="relative">
        <Box
          p={5}
          maxW="container.sm"
          mx="auto"
          height="100dvh"
          data-testid="feeds-skeleton-container"
        >
          <Flex direction="column" gap={4}>
            {/* Render 5 skeleton cards for immediate visual feedback */}
            {Array.from({ length: 5 }).map((_, index) => (
              <SkeletonFeedCard key={`skeleton-${index}`} />
            ))}
          </Flex>
        </Box>

        <FloatingMenu />
      </Box>
    );
  }

  // Show error state
  if (error) {
    return (
      <ErrorState error={error} onRetry={retryFetch} isLoading={isRetrying} />
    );
  }

  return (
    <Box minH="100dvh" position="relative">
      <Box
        aria-live="polite"
        aria-atomic="true"
        position="absolute"
        left="-10000px"
        width="1px"
        height="1px"
        overflow="hidden"
      >
        {liveRegionMessage}
      </Box>

      <Box
        ref={scrollContainerRef}
        p={5}
        maxW="container.sm"
        mx="auto"
        overflowY="auto"
        overflowX="hidden"
        height="100vh"
        data-testid="feeds-scroll-container"
        bg="var(--app-bg)"
      >
        {visibleFeeds.length > 0 ? (
          <>
            {/* Feed list rendering */}
            <VirtualFeedList
              feeds={visibleFeeds}
              readFeeds={readFeeds}
              onMarkAsRead={handleMarkAsRead}
            />

            {/* No more feeds indicator */}
            {!hasMore && visibleFeeds.length > 0 && (
              <Text
                textAlign="center"
                color="var(--alt-text-secondary)"
                fontSize="sm"
                mt={8}
                mb={4}
              >
                No more feeds to load
              </Text>
            )}
          </>
        ) : (
          /* Empty state - Use dedicated component for better UX */
          <EmptyFeedState />
        )}

        {/* Infinite scroll sentinel - always rendered when feeds are present and there's more to load */}
        {visibleFeeds.length > 0 && hasMore && (
          <div
            ref={sentinelRef}
            style={{
              height: "50px",
              width: "100%",
              backgroundColor: "transparent",
              margin: "10px 0",
              position: "relative",
              zIndex: 1,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              flexShrink: 0,
            }}
            data-testid="infinite-scroll-sentinel"
          >
            {/* Loading more indicator inside sentinel */}
            {isLoading && (
              <Text color="var(--alt-text-secondary)" fontSize="sm">
                Loading more...
              </Text>
            )}
          </div>
        )}
      </Box>

      {/* Swipe Mode Button */}
      <Link href="/mobile/feeds/swipe">
        <Box
          position="fixed"
          bottom={6}
          right={20}
          zIndex={1000}
          css={{
            paddingBottom: "calc(1.5rem + env(safe-area-inset-bottom, 0px))",
          }}
        >
          <Button
            data-testid="swipe-mode-button"
            size="md"
            borderRadius="full"
            bg="var(--alt-primary)"
            color="var(--text-primary)"
            p={0}
            w="48px"
            h="48px"
            border="2px solid white"
            _hover={{
              transform: "scale(1.05) rotate(90deg)",
              shadow: "0 6px 20px var(--alt-primary)",
              bg: "var(--alt-primary)",
            }}
            _active={{
              transform: "scale(0.95) rotate(90deg)",
            }}
            transition="all 0.3s cubic-bezier(0.4, 0, 0.2, 1)"
            tabIndex={0}
            role="button"
            aria-label="Open swipe mode"
            position="relative"
            overflow="hidden"
          >
            {/* Animated background pulse */}
            <Box
              position="absolute"
              top="50%"
              left="50%"
              transform="translate(-50%, -50%)"
              w="120%"
              h="120%"
              bg="var(--alt-primary)"
              borderRadius="full"
              opacity="0.3"
              css={{
                "@keyframes pulse": {
                  "0%, 100%": {
                    opacity: 0.6,
                    transform: "translate(-50%, -50%) scale(1)",
                  },
                  "50%": {
                    opacity: 0.8,
                    transform: "translate(-50%, -50%) scale(1.1)",
                  },
                },
                animation: "pulse 2s ease-in-out infinite",
              }}
            />
            <InfinityIcon size={20} style={{ position: "relative", zIndex: 1 }} />
          </Button>
        </Box>
      </Link>

      <FloatingMenu />
    </Box>
  );
}

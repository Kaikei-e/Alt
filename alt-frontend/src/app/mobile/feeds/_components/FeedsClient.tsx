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
import dynamic from "next/dynamic";
import VirtualFeedList from "@/components/mobile/VirtualFeedList";

// Dynamically import FloatingMenu to reduce initial bundle size for LCP optimization
const FloatingMenu = dynamic(
  () => import("@/components/mobile/utils/FloatingMenu").then((mod) => ({ default: mod.FloatingMenu })),
  { ssr: false }
);
import { useAuth } from "@/contexts/auth-context";
import { useCursorPagination } from "@/hooks/useCursorPagination";
import { feedApi } from "@/lib/api";
import { useInfiniteScroll } from "@/lib/utils/infiniteScroll";
import type { RenderFeed, SanitizedFeed } from "@/schema/feed";
import { toRenderFeed } from "@/schema/feed";
import ErrorState from "./ErrorState";

const PAGE_SIZE = 20;
const INITIAL_VISIBLE_CARDS = 3; // Limit initial render to 3 cards for LCP optimization
const STEP = 5; // Load 5 more cards when scrolling

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

interface FeedsClientProps {
  initialFeeds?: RenderFeed[];
}

export function FeedsClient({ initialFeeds = [] }: FeedsClientProps) {
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

  // Initialize readFeeds set from backend on mount using requestIdleCallback
  // Defer to avoid blocking LCP
  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }

    const initializeReadFeeds = async () => {
      try {
        // Fetch only the most recent read feeds for optimistic updates
        const readFeedsResponse = await feedApi.getReadFeedsWithCursor(undefined, 32);
        const readFeedLinks = new Set<string>();
        if (readFeedsResponse?.data) {
          readFeedsResponse.data.forEach((feed: SanitizedFeed) => {
            const canonical = canonicalize(feed.link);
            readFeedLinks.add(canonical);
          });
        }
        setReadFeeds(readFeedLinks);
      } catch (err) {
        // Continue with empty set if initialization fails
        // Backend filtering will still work correctly
        console.error("Failed to initialize read feeds:", err);
      }
    };

    // Use requestIdleCallback to defer initialization and avoid blocking LCP
    if ("requestIdleCallback" in window) {
      const idleCallbackId = window.requestIdleCallback(
        () => {
          void initializeReadFeeds();
        },
        { timeout: 2000 } // Fallback after 2s if idle never comes
      );
      return () => {
        window.cancelIdleCallback(idleCallbackId);
      };
    } else {
      // Fallback for browsers without requestIdleCallback
      const timeoutId = setTimeout(() => {
        void initializeReadFeeds();
      }, 100);
      return () => clearTimeout(timeoutId);
    }
  }, []);

  // Track visible count for progressive rendering
  const [visibleCount, setVisibleCount] = useState(INITIAL_VISIBLE_CARDS);

  // Use cursor-based pagination hook (only loads more after initial feeds are shown)
  const {
    data: feeds,
    hasMore,
    isLoading,
    error,
    isInitialLoading: hookIsInitialLoading,
    loadInitial,
    loadMore,
    refresh,
  } = useCursorPagination<SanitizedFeed>(feedApi.getFeedsWithCursor, {
    limit: PAGE_SIZE,
    autoLoad: false, // Don't auto-load initially, use initialFeeds first
  });

  // If we have initialFeeds, we're not in initial loading state
  // This prevents infinite loading spinner when initialFeeds are provided
  // IMPORTANT: This ensures server-rendered content matches client render
  // Also check if we have no feeds loaded yet (feeds is undefined or empty)
  const isInitialLoading = hookIsInitialLoading && initialFeeds.length === 0 && (!feeds || feeds.length === 0);

  // Ensure we have visible feeds to show (either from initialFeeds or loaded feeds)
  const hasVisibleContent = initialFeeds.length > 0 || (feeds && feeds.length > 0);

  // Merge initialFeeds with fetched feeds and filter/memoize visible feeds
  // Canonicalize feed.normalizedUrl (already normalized on server) to match readFeeds Set
  // Limit initial render to INITIAL_VISIBLE_CARDS for LCP optimization
  // IMPORTANT: Ensure consistent filtering between server and client to avoid hydration mismatch
  const visibleFeeds = useMemo(() => {
    // Start with initialFeeds (already RenderFeed[])
    const allFeeds: RenderFeed[] = [...initialFeeds];

    // Add fetched feeds (convert SanitizedFeed to RenderFeed)
    if (feeds) {
      const renderFeeds: RenderFeed[] = feeds.map((feed: SanitizedFeed) => toRenderFeed(feed));
      allFeeds.push(...renderFeeds);
    }

    // Filter out read feeds using normalizedUrl (already normalized on server)
    // Note: readFeeds starts as empty Set, so initial render matches server
    const filtered = allFeeds.filter((feed) => !readFeeds.has(feed.normalizedUrl));

    // For initial render, limit to visibleCount items to improve LCP
    // Additional items will be loaded progressively via IntersectionObserver
    // IMPORTANT: visibleCount starts at INITIAL_VISIBLE_CARDS (3) to match server expectation
    return filtered.slice(0, visibleCount);
  }, [initialFeeds, feeds, readFeeds, visibleCount]);

  // Handle marking feed as read with optimistic update + API call (TODO.mdの指示に基づく)
  const handleMarkAsRead = useCallback(async (rawLink: string) => {
    // Use normalizedUrl if available (from RenderFeed), otherwise canonicalize
    const link = rawLink.includes("?") || rawLink.includes("#")
      ? canonicalize(rawLink)
      : rawLink;

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

  // Start loading feeds after initial render
  // - If we have initialFeeds: load additional feeds in background for smooth scrolling
  // - If we have no initialFeeds: load feeds immediately (fallback for empty server response)
  useEffect(() => {
    // Only load if we haven't loaded feeds yet and there's more to load
    if (hasMore && !isLoading && (!feeds || feeds.length === 0)) {
      // If we have initialFeeds, defer loading to avoid blocking LCP
      // If we have no initialFeeds, load immediately to show content
      const shouldDefer = initialFeeds.length > 0;

      // Use loadInitial if we don't have a cursor yet (empty initialFeeds), otherwise loadMore
      const loadFn = initialFeeds.length === 0 ? loadInitial : loadMore;

      if (shouldDefer && "requestIdleCallback" in window) {
        const idleCallbackId = window.requestIdleCallback(
          () => {
            loadFn();
          },
          { timeout: 2000 }
        );
        return () => {
          window.cancelIdleCallback(idleCallbackId);
        };
      } else {
        const timeoutId = setTimeout(() => {
          loadFn();
        }, shouldDefer ? 500 : 100); // Faster for empty initialFeeds
        return () => clearTimeout(timeoutId);
      }
    }
  }, [initialFeeds.length, hasMore, isLoading, feeds, loadMore, loadInitial]);

  // Progressive rendering: increase visibleCount when user scrolls near the end
  useEffect(() => {
    if (typeof window === "undefined") return;

    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            // Load more cards progressively
            setVisibleCount((prev) => {
              const allFeedsCount = initialFeeds.length + (feeds?.length || 0);
              const nextCount = Math.min(prev + STEP, allFeedsCount);

              // If we've shown all initial feeds and need more, trigger API load
              if (nextCount >= initialFeeds.length && hasMore && !isLoading) {
                loadMore();
              }

              return nextCount;
            });
          }
        });
      },
      {
        rootMargin: "200px 0px", // Trigger earlier
        threshold: 0.1,
      }
    );

    if (sentinelRef.current) {
      observer.observe(sentinelRef.current);
    }

    return () => {
      observer.disconnect();
    };
  }, [initialFeeds.length, feeds?.length, hasMore, isLoading, loadMore]);

  // Use infinite scroll hook for additional pagination
  const handleLoadMore = useCallback(() => {
    if (hasMore && !isLoading) {
      loadMore();
    }
  }, [hasMore, isLoading, loadMore]);

  useInfiniteScroll(handleLoadMore, sentinelRef, visibleFeeds.length, {
    throttleDelay: 200,
    rootMargin: "100px 0px",
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
            {Array.from({ length: INITIAL_VISIBLE_CARDS }).map((_, index) => (
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

  // Show skeleton loading state only if we have no initialFeeds and no visible content
  // If initialFeeds exist, we should render them immediately to avoid hydration mismatch
  // Note: isInitialLoading is already adjusted above to be false when initialFeeds exist
  if (isInitialLoading && !hasVisibleContent) {
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
            {/* Render INITIAL_VISIBLE_CARDS skeleton cards for immediate visual feedback */}
            {Array.from({ length: INITIAL_VISIBLE_CARDS }).map((_, index) => (
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
    <Box minH="100dvh" position="relative" display="flex" flexDirection="column">
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
        flex="1"
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


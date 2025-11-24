import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import useSWRInfinite from "swr/infinite";
import { useArticleContentPrefetch } from "@/hooks/useArticleContentPrefetch";
import { feedApi } from "@/lib/api";
import type { CursorResponse } from "@/schema/common";
import type { RenderFeed, SanitizedFeed } from "@/schema/feed";
import { toRenderFeed } from "@/schema/feed";

// Use 20 for PAGE_SIZE to ensure stable pagination even with read filtering
// Reducing to 10 caused issues where all feeds in a page were filtered out,
// leading to immediate prefetch loops
const PAGE_SIZE = 20;
// Reduce PREFETCH_THRESHOLD to 5 to prefetch earlier, avoiding empty states
const PREFETCH_THRESHOLD = 5;
// Increase INITIAL_PAGE_COUNT to 2 to load more data upfront
const INITIAL_PAGE_COUNT = 2;
const EMPTY_PREFETCH_LIMIT = 3;

type SwrKey = readonly ["mobile-feed-swipe", string | undefined, number];

const canonicalize = (url: string) => {
  try {
    const parsed = new URL(url);
    // Remove fragment (hash)
    parsed.hash = "";

    // Remove tracking parameters (matching backend NormalizeURL)
    const trackingParams = [
      "utm_source",
      "utm_medium",
      "utm_campaign",
      "utm_term",
      "utm_content",
      "utm_id",
      "fbclid",
      "gclid",
      "mc_eid",
      "msclkid",
    ];
    trackingParams.forEach((param) => parsed.searchParams.delete(param));

    // Remove trailing slash (except for root path)
    if (parsed.pathname !== "/" && parsed.pathname.endsWith("/")) {
      parsed.pathname = parsed.pathname.slice(0, -1);
    }

    return parsed.toString();
  } catch {
    return url;
  }
};

const derivePageCursor = (
  pageData: CursorResponse<RenderFeed> | null,
): string | null => {
  if (!pageData) {
    return null;
  }
  if (pageData.next_cursor) {
    return pageData.next_cursor;
  }
  const lastFeed = pageData.data?.[pageData.data.length - 1];
  const published = lastFeed?.published?.trim();
  return published ? published : null;
};

const hasMorePages = (pageData: CursorResponse<RenderFeed> | null): boolean => {
  if (!pageData) {
    return false;
  }
  if (typeof pageData.has_more === "boolean") {
    return pageData.has_more;
  }
  return Boolean(derivePageCursor(pageData));
};

const createGetKey =
  (lastCursorRef: ReturnType<typeof useRef<string | null>>) =>
    (pageIndex: number, previousPageData: CursorResponse<RenderFeed> | null): SwrKey | null => {
      if (pageIndex === 0) {
        return ["mobile-feed-swipe", undefined, PAGE_SIZE];
      }

      if (previousPageData) {
        if (!hasMorePages(previousPageData)) {
          return null;
        }
        const cursor = derivePageCursor(previousPageData);
        // Always update lastCursorRef when we have a cursor
        if (cursor) {
          lastCursorRef.current = cursor;
        }

        // Use cursor if available, otherwise fallback to lastCursorRef
        const effectiveCursor = cursor || lastCursorRef.current || undefined;

        if (typeof window !== "undefined") {
          console.log("[useSwipeFeedController] getKey", {
            pageIndex,
            hasMore: hasMorePages(previousPageData),
            next_cursor: previousPageData.next_cursor,
            has_more: previousPageData.has_more,
            derivedCursor: cursor,
            lastCursorRef: lastCursorRef.current,
            effectiveCursor,
          });
        }
        return ["mobile-feed-swipe", effectiveCursor, PAGE_SIZE];
      }

      if (pageIndex > 0 && lastCursorRef.current) {
        if (typeof window !== "undefined") {
          console.log("[useSwipeFeedController] getKey (fallback)", {
            pageIndex,
            lastCursorRef: lastCursorRef.current,
          });
        }
        return ["mobile-feed-swipe", lastCursorRef.current, PAGE_SIZE];
      }

      if (typeof window !== "undefined") {
        console.warn("[useSwipeFeedController] getKey returned null", {
          pageIndex,
          previousPageData: previousPageData ? "exists" : "null",
          lastCursorRef: lastCursorRef.current,
        });
      }
      return null;
    };

const createFetchPage =
  (lastCursorRef: ReturnType<typeof useRef<string | null>>) =>
    async (
      _: string,
      cursor: string | undefined,
      limit: number,
    ): Promise<CursorResponse<RenderFeed>> => {
      // Use lastCursorRef as fallback if cursor is undefined
      // This ensures we always send a cursor when available, even if getKey didn't pass it
      const effectiveCursor = cursor || lastCursorRef.current || undefined;

      if (typeof window !== "undefined") {
        console.log("[useSwipeFeedController] fetchPage", {
          cursor,
          lastCursorRef: lastCursorRef.current,
          effectiveCursor,
          limit,
        });
      }

      const result = await feedApi.getFeedsWithCursor(effectiveCursor, limit);

      // Convert SanitizedFeed to RenderFeed
      const renderFeeds = result.data.map((feed: SanitizedFeed) => toRenderFeed(feed));

      if (typeof window !== "undefined") {
        console.log("[useSwipeFeedController] fetchPage result", {
          cursor,
          effectiveCursor,
          dataCount: renderFeeds.length,
          next_cursor: result.next_cursor,
          has_more: result.has_more,
        });
      }
      return {
        ...result,
        data: renderFeeds,
      };
    };

const clearTimeoutRef = (timeoutRef: ReturnType<typeof useRef<number | null>>) => {
  if (typeof window === "undefined") {
    timeoutRef.current = null;
    return;
  }
  if (timeoutRef.current) {
    window.clearTimeout(timeoutRef.current);
    timeoutRef.current = null;
  }
};

const scheduleTimeout = (
  timeoutRef: ReturnType<typeof useRef<number | null>>,
  callback: () => void,
  duration: number,
) => {
  if (typeof window === "undefined") {
    callback();
    return;
  }

  clearTimeoutRef(timeoutRef);
  timeoutRef.current = window.setTimeout(() => {
    timeoutRef.current = null;
    callback();
  }, duration);
};

export const useSwipeFeedController = (
  initialFeeds?: RenderFeed[] | null,
  initialNextCursor?: string,
) => {
  const [liveRegionMessage, setLiveRegionMessage] = useState("");
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const [readFeeds, setReadFeeds] = useState<Set<string>>(new Set());
  // Initialize to true if we have initialFeeds to avoid SSR/client mismatch
  // This ensures SWR can use fallbackData immediately on both server and client
  const [isReadFeedsInitialized, setIsReadFeedsInitialized] = useState(
    typeof window === "undefined" || Boolean(initialFeeds?.length),
  );
  const lastCursorRef = useRef<string | null>(initialNextCursor ?? null);
  const [isFeedSupplyDepleted, setIsFeedSupplyDepleted] = useState(false);
  const emptyPrefetchAttemptsRef = useRef(0);
  const [prefetchAttemptTick, setPrefetchAttemptTick] = useState(0);

  // Initialize readFeeds set from backend on mount using cursor-based pagination
  // Defer to requestIdleCallback to avoid blocking LCP
  // Only fetch recent read feeds (latest 32) for optimistic updates
  // Backend already filters out read feeds, so we don't need all read feeds
  useEffect(() => {
    if (typeof window === "undefined") {
      setIsReadFeedsInitialized(true);
      return;
    }

    const initializeReadFeeds = async () => {
      try {
        // Fetch only the most recent read feeds for optimistic updates
        // Reduced from 100 to 32 to improve initial load performance
        const readFeedsResponse = await feedApi.getReadFeedsWithCursor(
          undefined,
          32,
        );
        const readFeedLinks = new Set<string>();
        if (readFeedsResponse?.data) {
          readFeedsResponse.data.forEach((feed: SanitizedFeed) => {
            const canonical = canonicalize(feed.link);
            readFeedLinks.add(canonical);
          });
        }
        setReadFeeds(readFeedLinks);
        setIsReadFeedsInitialized(true);
      } catch (err) {
        // Continue with empty set if initialization fails
        // Backend filtering will still work correctly
        setIsReadFeedsInitialized(true);
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

  const liveRegionTimeoutRef = useRef<number | null>(null);
  const prefetchCursorRef = useRef<string | null>(null);
  const prefetchInFlightRef = useRef(false);

  // Wait for readFeeds initialization before fetching unread feeds
  // This ensures consistent behavior and prevents race conditions
  const getKey = useMemo(
    () => createGetKey(lastCursorRef),
    [lastCursorRef],
  );

  const fetchPage = useMemo(
    () => createFetchPage(lastCursorRef),
    [lastCursorRef],
  );

  const { data, error, isLoading, isValidating, setSize, mutate } =
    useSWRInfinite(
      isReadFeedsInitialized
        ? (pageIndex: number, previousPageData: CursorResponse<RenderFeed> | null) => {
          // If we have initial feeds and haven't fetched anything yet (pageIndex 0),
          // and we have a next cursor, we can skip the first fetch if we want to rely solely on initialFeeds.
          // However, SWR needs a key to return data.
          // Strategy:
          // If we have initialFeeds, we treat them as "page 0" data conceptually,
          // but SWR manages its own cache.
          // To delay fetching, we can return null for the first page key until we need more data.
          // BUT, we want SWR to manage the subsequent pages.

          // Simplified approach:
          // Let SWR fetch normally, but we use initialFeeds for immediate display.
          // To truly delay the fetch, we could use a state `shouldFetch` initialized to false if initialFeeds exist.

          // Fix cursor=null issue: SWR may not pass previousPageData correctly in some cases
          // Use the getKey function but ensure we handle null previousPageData by using lastCursorRef
          if (pageIndex === 0) {
            // If we have initial feeds, we might want to delay the first fetch?
            // Actually, the requirement is "fetch subsequent feeds when user approaches end".
            // So we can start with page 0 being the initial feeds (if we could inject them into SWR).
            // SWR doesn't easily support "injecting" initial data for infinite loading without `fallbackData` which is static.

            // For now, let's keep the standard fetching but use initialFeeds for display.
            // The optimization requested is "Delay fetching of subsequent feeds".
            // Since we already have 5 feeds, we can delay the fetch of the *next* batch (20).
            // But SWR will try to fetch page 0 immediately.

            // To prevent immediate fetch of page 0 (which would be the 20 items),
            // we can return null if we are satisfied with initialFeeds and haven't reached the end.
            // But that complicates the "load more" logic.

            return getKey(pageIndex, previousPageData);
          }

          // If previousPageData is null but we have a cursor in lastCursorRef, use it
          if (!previousPageData && lastCursorRef.current) {
            if (typeof window !== "undefined") {
              console.log("[useSwipeFeedController] getKey: using lastCursorRef fallback", {
                pageIndex,
                lastCursorRef: lastCursorRef.current,
              });
            }
            return ["mobile-feed-swipe", lastCursorRef.current, PAGE_SIZE];
          }

          return getKey(pageIndex, previousPageData);
        }
        : () => null,
      fetchPage,
      {
        revalidateOnFocus: false,
        revalidateFirstPage: false,
        parallel: false, // Set to false to ensure sequential fetching and proper previousPageData passing
        persistSize: true, // Prevent page size reset when first page key changes (prevents cursor=null issue)
        initialSize: initialFeeds && initialFeeds.length > 0 ? 1 : INITIAL_PAGE_COUNT,
        fallbackData: initialFeeds && initialFeeds.length > 0 ? [{
          data: initialFeeds,
          next_cursor: initialNextCursor ?? null,
          has_more: !!initialNextCursor,
        }] : undefined,
      },
    );

  const feeds = useMemo(() => {
    if (!data || data.length === 0) {
      // Fallback to initialFeeds if SWR has no data yet
      if (initialFeeds && initialFeeds.length > 0) {
        // Check if initial feed is read (though unlikely for server-fetched data)
        if (readFeeds.size > 0) {
          return initialFeeds.filter(feed => !readFeeds.has(canonicalize(feed.link)));
        }
        return initialFeeds;
      }
      return [] as RenderFeed[];
    }

    const allFeeds = data.flatMap((page) => page?.data ?? []);

    if (readFeeds.size === 0) {
      return allFeeds;
    }

    return allFeeds.filter(
      (feed) => !readFeeds.has(canonicalize(feed.link)),
    );
  }, [data, readFeeds, initialFeeds]);

  const activeFeed = feeds[0] ?? null;
  const activeIndex = activeFeed ? 0 : -1;
  const lastPage = data?.[data.length - 1] ?? null;
  const hasMoreFromServer = Boolean(
    lastPage?.has_more ?? Boolean(lastPage?.next_cursor),
  );
  const hasMore = hasMoreFromServer && !isFeedSupplyDepleted;
  // If we have feeds (from data or initialFeeds), we're not in initial loading state
  // Also check if we have initialFeeds to avoid showing loading when data is available from SSR
  const isInitialLoading = feeds.length === 0 && isLoading && !initialFeeds?.length;

  // Article content prefetch hook
  const { triggerPrefetch, getCachedContent, markAsDismissed } =
    useArticleContentPrefetch(
      feeds,
      Math.max(activeIndex, 0),
      2, // Prefetch next 2 articles
    );

  useEffect(() => {
    if (!statusMessage) {
      return;
    }

    if (typeof window === "undefined") {
      setStatusMessage(null);
      return;
    }

    const timeout = window.setTimeout(() => {
      setStatusMessage(null);
    }, 2000);

    return () => window.clearTimeout(timeout);
  }, [statusMessage]);

  useEffect(() => {
    return () => {
      clearTimeoutRef(liveRegionTimeoutRef);
    };
  }, []);

  const announce = useCallback((message: string, duration: number) => {
    setLiveRegionMessage(message);
    scheduleTimeout(
      liveRegionTimeoutRef,
      () => {
        setLiveRegionMessage("");
      },
      duration,
    );
  }, []);

  // Reset empty prefetch attempts when feeds are available or hasMoreFromServer is false
  useEffect(() => {
    if (!hasMoreFromServer || feeds.length > 0) {
      emptyPrefetchAttemptsRef.current = 0;
      prefetchInFlightRef.current = false;
      if (isFeedSupplyDepleted) {
        setIsFeedSupplyDepleted(false);
      }
    }
  }, [feeds.length, hasMoreFromServer, isFeedSupplyDepleted]);

  // Reset prefetchCursorRef when validation completes to allow retry
  // This is critical when feeds.length === 0 but hasMore === true,
  // as we need to retry fetching the next page after validation completes
  useEffect(() => {
    if (!isValidating && prefetchCursorRef.current !== null) {
      if (feeds.length === 0 && hasMoreFromServer) {
        prefetchCursorRef.current = null;
      }
    }
  }, [isValidating, feeds.length, hasMoreFromServer]);

  // Check if feed supply is depleted after each prefetch attempt
  // This ensures isFeedSupplyDepleted is set synchronously after emptyPrefetchAttemptsRef is incremented
  useEffect(() => {
    if (feeds.length === 0 && hasMoreFromServer && !isValidating) {
      if (emptyPrefetchAttemptsRef.current >= EMPTY_PREFETCH_LIMIT) {
        setIsFeedSupplyDepleted(true);
      }
    }
  }, [feeds.length, hasMoreFromServer, isValidating, prefetchAttemptTick]);

  // Memoize prefetch logic to avoid unnecessary recalculations
  const schedulePrefetch = useCallback(() => {
    if (!lastPage || !data) {
      prefetchCursorRef.current = null;
      return;
    }

    const nextCursor = derivePageCursor(lastPage);
    if (!nextCursor) {
      prefetchCursorRef.current = null;
      return;
    }
    lastCursorRef.current = nextCursor;

    // If feeds array is empty but hasMoreFromServer is true, we should prefetch immediately
    // This handles the case where all feeds in current pages are filtered out
    // However, we must check isValidating to avoid infinite prefetch loops
    if (feeds.length === 0) {
      // Check for feed supply depletion first
      if (emptyPrefetchAttemptsRef.current >= EMPTY_PREFETCH_LIMIT) {
        setIsFeedSupplyDepleted(true);
        prefetchCursorRef.current = null;
        prefetchInFlightRef.current = false;
        return;
      }

      if (
        !isValidating &&
        prefetchCursorRef.current !== nextCursor &&
        !prefetchInFlightRef.current &&
        hasMoreFromServer
      ) {
        prefetchCursorRef.current = nextCursor;
        prefetchInFlightRef.current = true;
        // Use requestIdleCallback to defer prefetch and avoid blocking main thread
        if (typeof window !== "undefined" && "requestIdleCallback" in window) {
          window.requestIdleCallback(
            () => {
              emptyPrefetchAttemptsRef.current += 1;
              setSize((current) => current + 1);
              setPrefetchAttemptTick((tick) => tick + 1);
              if (process.env.NODE_ENV === "test") {
                console.log("[SwipeFeedController] attempt", emptyPrefetchAttemptsRef.current);
              }
              prefetchInFlightRef.current = false;
            },
            { timeout: 1000 }
          );
        } else {
          emptyPrefetchAttemptsRef.current += 1;
          setSize((current) => current + 1);
          setPrefetchAttemptTick((tick) => tick + 1);
          if (process.env.NODE_ENV === "test") {
            console.log("[SwipeFeedController] attempt", emptyPrefetchAttemptsRef.current);
          }
          prefetchInFlightRef.current = false;
        }
      }
      return;
    }

    // Only check hasMore for non-empty feeds
    if (!hasMore) {
      prefetchCursorRef.current = null;
      return;
    }

    const remainingAfterCurrent = Math.max(feeds.length - (activeFeed ? 1 : 0), 0);

    const totalRawFeeds = data.flatMap((page) => page?.data ?? []).length;
    const filterRatio =
      totalRawFeeds > 0 ? Math.min(feeds.length / totalRawFeeds, 1) : 1;
    const adjustedThreshold = Math.max(
      1,
      Math.ceil(PREFETCH_THRESHOLD * filterRatio),
    );

    if (
      remainingAfterCurrent <= adjustedThreshold &&
      !isValidating &&
      prefetchCursorRef.current !== nextCursor
    ) {
      if (prefetchInFlightRef.current) {
        return;
      }
      prefetchCursorRef.current = nextCursor;
      prefetchInFlightRef.current = true;
      // Use requestIdleCallback to defer prefetch and avoid blocking main thread
      if (typeof window !== "undefined" && "requestIdleCallback" in window) {
        window.requestIdleCallback(
          () => {
            setSize((current) => current + 1);
            prefetchInFlightRef.current = false;
          },
          { timeout: 1000 }
        );
      } else {
        setSize((current) => current + 1);
        prefetchInFlightRef.current = false;
      }
    }
  }, [hasMore, lastPage, data, feeds.length, activeFeed, isValidating, setSize]);

  useEffect(() => {
    schedulePrefetch();
  }, [schedulePrefetch, feeds.length, prefetchAttemptTick]);

  // Preload next feed image using requestIdleCallback to avoid blocking main thread
  useEffect(() => {
    if (feeds.length > 1 && typeof window !== "undefined") {
      const nextFeed = feeds[1];
      let linkElement: HTMLLinkElement | null = null;

      const preloadImage = () => {
        // Extract image from description or content if possible
        // Since we don't have a direct image field, we try to parse description
        const parser = new DOMParser();
        const doc = parser.parseFromString(nextFeed.description || "", "text/html");
        const img = doc.querySelector("img");
        if (img && img.src) {
          linkElement = document.createElement("link");
          linkElement.rel = "preload";
          linkElement.as = "image";
          linkElement.href = img.src;
          document.head.appendChild(linkElement);
        }
      };

      // Use requestIdleCallback to defer image preloading
      if ("requestIdleCallback" in window) {
        const idleCallbackId = window.requestIdleCallback(preloadImage, {
          timeout: 2000,
        });
        return () => {
          window.cancelIdleCallback(idleCallbackId);
          if (linkElement && document.head.contains(linkElement)) {
            document.head.removeChild(linkElement);
          }
        };
      } else {
        // Fallback for browsers without requestIdleCallback
        const timeoutId = setTimeout(preloadImage, 100);
        return () => {
          clearTimeout(timeoutId);
          if (linkElement && document.head.contains(linkElement)) {
            document.head.removeChild(linkElement);
          }
        };
      }
    }
  }, [feeds]);

  // Trigger article content prefetch when active index changes
  // This ensures prefetch happens AFTER dismiss and mutate complete
  useEffect(() => {
    if (activeIndex >= 0 && feeds.length > 0) {
      triggerPrefetch();
    }
  }, [activeIndex, feeds.length, triggerPrefetch]);

  const dismissActiveFeed = useCallback(
    async (_direction: number) => {
      const current = activeFeed;

      if (!current) {
        return;
      }

      const canonicalLink = canonicalize(current.link);

      // Check if already marked as read (prevent duplicate requests)
      if (readFeeds.has(canonicalLink)) {
        // Still proceed with UI update
      }

      setStatusMessage("Feed marked as read");
      announce("Feed marked as read", 1000);

      // Mark article as dismissed BEFORE API call to prevent prefetch race condition
      markAsDismissed(canonicalLink);

      // Optimistic update: add to readFeeds Set immediately
      setReadFeeds((prev) => {
        const next = new Set(prev);
        next.add(canonicalLink);
        return next;
      });

      try {
        await feedApi.updateFeedReadStatus(canonicalLink);

        // Mutate cache to remove dismissed feed immediately
        await mutate(
          (currentData) => {
            if (!currentData) {
              return currentData;
            }
            const filtered = currentData.map((page) => {
              if (!page?.data) return page;
              const filteredData = page.data.filter(
                (feed) => canonicalize(feed.link) !== canonicalLink,
              );
              return {
                ...page,
                data: filteredData,
              };
            });
            return filtered;
          },
          { revalidate: false, populateCache: true },
        );
      } catch (err) {
        // Rollback optimistic update on error
        setReadFeeds((prev) => {
          const next = new Set(prev);
          next.delete(canonicalLink);
          return next;
        });
        setStatusMessage("Failed to mark feed as read");
        announce("Failed to mark feed as read", 1500);
        throw err;
      }
    },
    [activeFeed, announce, markAsDismissed, mutate, readFeeds],
  );

  const retry = useCallback(async () => {
    emptyPrefetchAttemptsRef.current = 0;
    prefetchInFlightRef.current = false;
    setIsFeedSupplyDepleted(false);
    prefetchCursorRef.current = null;
    await mutate(undefined, { revalidate: true });
  }, [mutate]);

  return {
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
    getCachedContent,
  };
};

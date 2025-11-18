import {
  type MutableRefObject,
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
import type { Feed } from "@/schema/feed";

const PAGE_SIZE = 20;
const PREFETCH_THRESHOLD = 10;
const INITIAL_PAGE_COUNT = 3;

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
  pageData: CursorResponse<Feed> | null,
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

const hasMorePages = (pageData: CursorResponse<Feed> | null): boolean => {
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
    (pageIndex: number, previousPageData: CursorResponse<Feed> | null): SwrKey | null => {
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
    ): Promise<CursorResponse<Feed>> => {
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

      if (typeof window !== "undefined") {
        console.log("[useSwipeFeedController] fetchPage result", {
          cursor,
          effectiveCursor,
          dataCount: result.data.length,
          next_cursor: result.next_cursor,
          has_more: result.has_more,
        });
      }
      return result;
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

export const useSwipeFeedController = () => {
  const [liveRegionMessage, setLiveRegionMessage] = useState("");
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const [readFeeds, setReadFeeds] = useState<Set<string>>(new Set());
  const [isReadFeedsInitialized, setIsReadFeedsInitialized] = useState(false);
  const lastCursorRef = useRef<string | null>(null);

  // Initialize readFeeds set from backend on mount using cursor-based pagination
  // Only fetch recent read feeds (latest 100) for optimistic updates
  // Backend already filters out read feeds, so we don't need all read feeds
  useEffect(() => {
    const initializeReadFeeds = async () => {
      try {
        // Fetch only the most recent read feeds for optimistic updates
        // This is sufficient since backend already excludes read feeds from unread feed queries
        const readFeedsResponse = await feedApi.getReadFeedsWithCursor(
          undefined,
          100,
        );
        const readFeedLinks = new Set<string>();
        if (readFeedsResponse?.data) {
          readFeedsResponse.data.forEach((feed: Feed) => {
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

    void initializeReadFeeds();
  }, []);

  const liveRegionTimeoutRef = useRef<number | null>(null);
  const prefetchCursorRef = useRef<string | null>(null);

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
        ? (pageIndex: number, previousPageData: CursorResponse<Feed> | null) => {
          // Fix cursor=null issue: SWR may not pass previousPageData correctly in some cases
          // Use the getKey function but ensure we handle null previousPageData by using lastCursorRef
          if (pageIndex === 0) {
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
        initialSize: INITIAL_PAGE_COUNT,
      },
    );

  const feeds = useMemo(() => {
    if (!data || data.length === 0) {
      return [] as Feed[];
    }

    const allFeeds = data.flatMap((page) => page?.data ?? []);

    if (readFeeds.size === 0) {
      return allFeeds;
    }

    return allFeeds.filter(
      (feed) => !readFeeds.has(canonicalize(feed.link)),
    );
  }, [data, readFeeds]);

  const activeFeed = feeds[0] ?? null;
  const activeIndex = activeFeed ? 0 : -1;
  const lastPage = data?.[data.length - 1] ?? null;
  const hasMore = Boolean(
    lastPage?.has_more ?? Boolean(lastPage?.next_cursor),
  );
  const isInitialLoading = (!data || data.length === 0) && isLoading;

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

  const schedulePrefetch = useCallback(() => {
    if (!hasMore || !lastPage || !data) {
      if (typeof window !== "undefined") {
        console.log("[useSwipeFeedController] schedulePrefetch: early return", {
          hasMore,
          hasLastPage: !!lastPage,
          hasData: !!data,
        });
      }
      prefetchCursorRef.current = null;
      return;
    }

    const nextCursor = derivePageCursor(lastPage);
    if (!nextCursor) {
      if (typeof window !== "undefined") {
        console.warn("[useSwipeFeedController] schedulePrefetch: no cursor derived", {
          lastPage: {
            has_more: lastPage.has_more,
            next_cursor: lastPage.next_cursor,
            dataCount: lastPage.data?.length ?? 0,
          },
        });
      }
      prefetchCursorRef.current = null;
      return;
    }
    lastCursorRef.current = nextCursor;

    // If feeds array is empty but hasMore is true, we should prefetch immediately
    // This handles the case where all feeds in current pages are filtered out
    if (feeds.length === 0) {
      if (prefetchCursorRef.current !== nextCursor) {
        if (typeof window !== "undefined") {
          console.log("[useSwipeFeedController] schedulePrefetch: empty feeds, prefetching", {
            nextCursor,
            currentSize: data.length,
          });
        }
        prefetchCursorRef.current = nextCursor;
        setSize((current) => current + 1);
      }
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
      if (typeof window !== "undefined") {
        console.log("[useSwipeFeedController] schedulePrefetch: triggering prefetch", {
          nextCursor,
          remainingAfterCurrent,
          adjustedThreshold,
          isValidating,
          currentSize: data.length,
        });
      }
      prefetchCursorRef.current = nextCursor;
      setSize((current) => current + 1);
    } else if (typeof window !== "undefined") {
      console.log("[useSwipeFeedController] schedulePrefetch: conditions not met", {
        remainingAfterCurrent,
        adjustedThreshold,
        isValidating,
        prefetchCursorRef: prefetchCursorRef.current,
        nextCursor,
      });
    }
  }, [activeFeed, data, feeds.length, hasMore, isValidating, lastPage, setSize]);

  useEffect(() => {
    schedulePrefetch();
  }, [schedulePrefetch, feeds.length]);

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

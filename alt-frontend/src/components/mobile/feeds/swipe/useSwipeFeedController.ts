import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type MutableRefObject,
} from "react";
import useSWRInfinite from "swr/infinite";
import { feedsApi } from "@/lib/api";
import { CursorResponse } from "@/schema/common";
import { Feed } from "@/schema/feed";
import { useArticleContentPrefetch } from "@/hooks/useArticleContentPrefetch";

const PAGE_SIZE = 20;
const PREFETCH_THRESHOLD = 10;
const INITIAL_PAGE_COUNT = 3;

type SwrKey = readonly ["mobile-feed-swipe", string | undefined, number];

const canonicalize = (url: string) => {
  try {
    const parsed = new URL(url);
    parsed.hash = "";
    [
      "utm_source",
      "utm_medium",
      "utm_campaign",
      "utm_term",
      "utm_content",
    ].forEach((param) => parsed.searchParams.delete(param));
    if (parsed.pathname !== "/" && parsed.pathname.endsWith("/")) {
      parsed.pathname = parsed.pathname.slice(0, -1);
    }
    return parsed.toString();
  } catch {
    return url;
  }
};

const getKey = (
  pageIndex: number,
  previousPageData: CursorResponse<Feed> | null,
): SwrKey | null => {
  if (previousPageData && !previousPageData.next_cursor) {
    return null;
  }

  if (pageIndex === 0) {
    return ["mobile-feed-swipe", undefined, PAGE_SIZE];
  }

  const cursor = previousPageData?.next_cursor ?? undefined;
  return ["mobile-feed-swipe", cursor, PAGE_SIZE];
};

const fetchPage = async (
  _: string,
  cursor: string | undefined,
  limit: number,
): Promise<CursorResponse<Feed>> => {
  return feedsApi.getFeedsWithCursor(cursor, limit);
};

const clearTimeoutRef = (timeoutRef: MutableRefObject<number | null>) => {
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
  timeoutRef: MutableRefObject<number | null>,
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
  const [activeFeedId, setActiveFeedId] = useState<string | null>(null);
  const [readFeeds, setReadFeeds] = useState<Set<string>>(new Set());

  const liveRegionTimeoutRef = useRef<number | null>(null);
  const prefetchCursorRef = useRef<string | null>(null);
  const lastDismissedIdRef = useRef<string | null>(null);

  const { data, error, isLoading, isValidating, setSize, mutate } =
    useSWRInfinite(getKey, fetchPage, {
      revalidateOnFocus: false,
      revalidateFirstPage: false,
      parallel: true,
      initialSize: INITIAL_PAGE_COUNT,
    });

  const feeds = useMemo(() => {
    if (!data || data.length === 0) {
      return [] as Feed[];
    }
    const allFeeds = data.flatMap((page) => page?.data ?? []);
    // Filter out read feeds using optimistic update Set
    return allFeeds.filter((feed) => !readFeeds.has(canonicalize(feed.link)));
  }, [data, readFeeds]);

  const activeIndex = useMemo(() => {
    if (feeds.length === 0) {
      return 0;
    }

    if (!activeFeedId) {
      return 0;
    }

    const index = feeds.findIndex((feed) => feed.id === activeFeedId);
    return index === -1 ? 0 : index;
  }, [activeFeedId, feeds]);

  const activeFeed = feeds[activeIndex] ?? null;
  const lastPage = data?.[data.length - 1] ?? null;
  const hasMore = Boolean(lastPage?.next_cursor);
  const isInitialLoading = (!data || data.length === 0) && isLoading;

  // Article content prefetch hook
  const { triggerPrefetch, getCachedContent, markAsDismissed } =
    useArticleContentPrefetch(
      feeds,
      activeIndex,
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

  useEffect(() => {
    if (feeds.length === 0) {
      if (activeFeedId !== null) {
        setActiveFeedId(null);
      }
      lastDismissedIdRef.current = null;
      return;
    }

    const hasActiveFeed =
      activeFeedId !== null && feeds.some((feed) => feed.id === activeFeedId);

    if (hasActiveFeed) {
      if (
        lastDismissedIdRef.current &&
        feeds.every((feed) => feed.id !== lastDismissedIdRef.current)
      ) {
        lastDismissedIdRef.current = null;
      }
      return;
    }

    if (
      lastDismissedIdRef.current &&
      feeds.some((feed) => feed.id === lastDismissedIdRef.current)
    ) {
      return;
    }

    setActiveFeedId(feeds[0].id);
  }, [activeFeedId, feeds]);

  const announce = useCallback(
    (message: string, duration: number) => {
      setLiveRegionMessage(message);
      scheduleTimeout(liveRegionTimeoutRef, () => {
        setLiveRegionMessage("");
      }, duration);
    },
    [],
  );

  const schedulePrefetch = useCallback(() => {
    if (!hasMore || !lastPage) {
      prefetchCursorRef.current = null;
      return;
    }

    const nextCursor = lastPage.next_cursor;
    const remaining = feeds.length - activeIndex;

    if (
      nextCursor &&
      remaining <= PREFETCH_THRESHOLD &&
      remaining >= 0 &&
      !isValidating &&
      prefetchCursorRef.current !== nextCursor
    ) {
      prefetchCursorRef.current = nextCursor;
      setSize((current) => current + 1);
    }
  }, [activeIndex, feeds.length, hasMore, isValidating, lastPage, setSize]);

  useEffect(() => {
    schedulePrefetch();
  }, [schedulePrefetch, activeIndex, feeds.length]);

  // Trigger article content prefetch when active index changes
  // This ensures prefetch happens AFTER dismiss and mutate complete
  useEffect(() => {
    if (activeIndex >= 0 && feeds.length > 0) {
      triggerPrefetch();
    }
  }, [activeIndex, feeds.length, triggerPrefetch]);

  const dismissActiveFeed = useCallback(
    async (_direction: number) => {
      const currentIndex =
        activeFeedId !== null
          ? feeds.findIndex((feed) => feed.id === activeFeedId)
          : 0;
      const resolvedIndex = currentIndex === -1 ? 0 : currentIndex;
      const current = feeds[resolvedIndex];

      if (!current) {
        return;
      }

      const nextFeed = feeds[resolvedIndex + 1] ?? null;
      lastDismissedIdRef.current = null;

      if (nextFeed) {
        setActiveFeedId(nextFeed.id);
      } else {
        lastDismissedIdRef.current = current.id;
        setActiveFeedId(null);
      }

      setStatusMessage("Feed marked as read");
      announce("Feed marked as read", 1000);

      // Mark article as dismissed BEFORE API call to prevent prefetch race condition
      const canonicalLink = canonicalize(current.link);
      markAsDismissed(canonicalLink);

      // Optimistic update: add to readFeeds Set immediately
      setReadFeeds((prev) => new Set(prev).add(canonicalLink));

      try {
        await feedsApi.updateFeedReadStatus(canonicalLink);
        await mutate();

        // Prefetch is now triggered by activeIndex useEffect (lines 234-238)
        // This prevents race condition between read status update and prefetch
      } catch (err) {
        console.error("Failed to mark feed as read", err);
        // Rollback optimistic update on error
        setReadFeeds((prev) => {
          const next = new Set(prev);
          next.delete(canonicalLink);
          return next;
        });
        setActiveFeedId(current.id);
        lastDismissedIdRef.current = null;
        setStatusMessage("Failed to mark feed as read");
        announce("Failed to mark feed as read", 1500);
        throw err;
      }
    },
    [activeFeedId, announce, feeds, markAsDismissed, mutate],
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

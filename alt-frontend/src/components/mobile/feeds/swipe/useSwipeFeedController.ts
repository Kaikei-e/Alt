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
  const [activeIndex, setActiveIndex] = useState(0);

  const liveRegionTimeoutRef = useRef<number | null>(null);
  const prefetchCursorRef = useRef<string | null>(null);

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
    return data.flatMap((page) => page?.data ?? []);
  }, [data]);

  const activeFeed = feeds[activeIndex] ?? null;
  const lastPage = data?.[data.length - 1] ?? null;
  const hasMore = Boolean(lastPage?.next_cursor);
  const isInitialLoading = (!data || data.length === 0) && isLoading;

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
    if (feeds.length === 0 && activeIndex !== 0) {
      setActiveIndex(0);
    }
  }, [feeds.length, activeIndex]);

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

  const dismissActiveFeed = useCallback(
    async (_direction: number) => {
      const current = feeds[activeIndex];
      if (!current) {
        return;
      }

      setActiveIndex((prev) => prev + 1);
      setStatusMessage("Feed marked as read");
      announce("Feed marked as read", 1000);

      try {
        const canonicalLink = canonicalize(current.link);
        await feedsApi.updateFeedReadStatus(canonicalLink);
        await mutate();
      } catch (err) {
        console.error("Failed to mark feed as read", err);
        setActiveIndex((prev) => Math.max(prev - 1, 0));
        setStatusMessage("Failed to mark feed as read");
        announce("Failed to mark feed as read", 1500);
        throw err;
      }
    },
    [activeIndex, announce, feeds, mutate],
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
  };
};

"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { Feed } from "@/schema/feed";
import { feedsApi } from "@/lib/api";

export interface UseFavoriteFeedsResult {
  feeds: Feed[];
  isLoading: boolean;
  error: Error | null;
  hasMore: boolean;
  loadMore: () => void;
  refresh: () => void;
}

export const useFavoriteFeeds = (
  initialLimit: number = 20,
): UseFavoriteFeedsResult => {
  const enablePrefetch = true;

  const [feeds, setFeeds] = useState<Feed[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [hasMore, setHasMore] = useState(true);
  const [cursor, setCursor] = useState<string | undefined>(undefined);

  const prefetchCacheRef = useRef<Map<string, unknown>>(new Map());
  const prefetchTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  const prefetchNextPage = useCallback(
    async (nextCursor: string) => {
      if (!enablePrefetch) return;

      if (prefetchCacheRef.current.has(nextCursor)) {
        return;
      }

      try {
        prefetchCacheRef.current.set(nextCursor, "loading");

        const response = await feedsApi.getFavoriteFeedsWithCursor(
          nextCursor,
          initialLimit,
        );

        prefetchCacheRef.current.set(nextCursor, response);

        if (prefetchCacheRef.current.size > 3) {
          const entries = Array.from(prefetchCacheRef.current.keys());
          const oldestKey = entries[0];
          prefetchCacheRef.current.delete(oldestKey);
        }
      } catch {
        prefetchCacheRef.current.delete(nextCursor);
      }
    },
    [initialLimit, enablePrefetch],
  );

  const loadFeeds = useCallback(
    async (resetData: boolean = false) => {
      try {
        setIsLoading(true);
        setError(null);

        const currentCursor = resetData ? undefined : cursor;
        let response: { data: Feed[]; next_cursor: string | null } | undefined;

        if (
          enablePrefetch &&
          currentCursor &&
          prefetchCacheRef.current.has(currentCursor)
        ) {
          const cachedResponse = prefetchCacheRef.current.get(currentCursor);
          if (cachedResponse !== "loading") {
            response = cachedResponse as {
              data: Feed[];
              next_cursor: string | null;
            };
            prefetchCacheRef.current.delete(currentCursor);
          }
        }

        if (!response) {
          response = await feedsApi.getFavoriteFeedsWithCursor(
            currentCursor,
            initialLimit,
          );
        }

        if (resetData) {
          setFeeds(response.data);
        } else {
          setFeeds((prevFeeds) => [...prevFeeds, ...response.data]);
        }

        setCursor(response.next_cursor || undefined);
        setHasMore(response.next_cursor !== null);

        if (enablePrefetch && response.next_cursor) {
          if (prefetchTimeoutRef.current) {
            clearTimeout(prefetchTimeoutRef.current);
          }
          prefetchTimeoutRef.current = setTimeout(() => {
            prefetchNextPage(response!.next_cursor!);
          }, 500);
        }
      } catch (err) {
        setError(err as Error);
        setHasMore(false);
        if (resetData) {
          setFeeds([]);
        }
      } finally {
        setIsLoading(false);
      }
    },
    [cursor, initialLimit, prefetchNextPage, enablePrefetch],
  );

  const loadMore = useCallback(() => {
    if (!isLoading && hasMore && cursor) {
      loadFeeds(false);
    }
  }, [isLoading, hasMore, cursor, loadFeeds]);

  const refresh = useCallback(async () => {
    prefetchCacheRef.current.clear();
    if (prefetchTimeoutRef.current) {
      clearTimeout(prefetchTimeoutRef.current);
    }

    setCursor(undefined);
    setHasMore(true);
    await loadFeeds(true);
  }, [loadFeeds]);

  useEffect(() => {
    const initialLoad = async () => {
      try {
        setIsLoading(true);
        setError(null);

        const response = await feedsApi.getFavoriteFeedsWithCursor(
          undefined,
          initialLimit,
        );
        setFeeds(response.data);
        setCursor(response.next_cursor || undefined);
        setHasMore(response.next_cursor !== null);

        if (enablePrefetch && response.next_cursor) {
          prefetchTimeoutRef.current = setTimeout(() => {
            prefetchNextPage(response.next_cursor!);
          }, 500);
        }
      } catch (err) {
        setError(err as Error);
        setHasMore(false);
        setFeeds([]);
      } finally {
        setIsLoading(false);
      }
    };

    initialLoad();
  }, [initialLimit, prefetchNextPage, enablePrefetch]);

  useEffect(() => {
    const cache = prefetchCacheRef.current;
    const timeout = prefetchTimeoutRef.current;

    return () => {
      if (timeout) {
        clearTimeout(timeout);
      }
      cache.clear();
    };
  }, []);

  return {
    feeds,
    isLoading,
    error,
    hasMore,
    loadMore,
    refresh,
  };
};

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/api/core/ApiError";
import type { UsePaginationResult } from "@/schema/common";

export interface UseCursorPaginationOptions {
  limit?: number;
  enablePrefetch?: boolean;
  prefetchDelay?: number;
  autoLoad?: boolean;
}

// Use the common UsePaginationResult directly
export type UseCursorPaginationResult<T> = UsePaginationResult<T>;

export function useCursorPagination<T>(
  fetchFn: (cursor?: string, limit?: number) => Promise<{ data: T[]; next_cursor: string | null }>,
  options: UseCursorPaginationOptions = {}
): UseCursorPaginationResult<T> {
  const { limit = 20, enablePrefetch = false, prefetchDelay = 500, autoLoad = false } = options;

  const [data, setData] = useState<T[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(true);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [isInitialLoading, setIsInitialLoading] = useState(true);

  // Prefetch cache and refs
  const prefetchCacheRef = useRef<Map<string, unknown>>(new Map());
  const prefetchTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  // Background prefetch function
  const prefetchNextPage = useCallback(
    async (nextCursor: string) => {
      if (!enablePrefetch || prefetchCacheRef.current.has(nextCursor)) {
        return; // Not enabled or already prefetching/cached
      }

      try {
        // Mark as being prefetched
        prefetchCacheRef.current.set(nextCursor, "loading");

        const response = await fetchFn(nextCursor, limit);

        // Cache the response
        prefetchCacheRef.current.set(nextCursor, response);

        // Clean up old cache entries (keep only last 3)
        if (prefetchCacheRef.current.size > 3) {
          const entries = Array.from(prefetchCacheRef.current.keys());
          const oldestKey = entries[0];
          prefetchCacheRef.current.delete(oldestKey);
        }
      } catch {
        // Remove failed prefetch attempt
        prefetchCacheRef.current.delete(nextCursor);
      }
    },
    [enablePrefetch, fetchFn, limit]
  );

  const loadInitial = useCallback(async () => {
    setIsInitialLoading(true);
    setIsLoading(true);
    setError(null);

    try {
      const response = await fetchFn(undefined, limit);
      setData(response.data);
      setCursor(response.next_cursor);
      setHasMore(response.next_cursor !== null);

      // Prefetch next page in background if available
      if (enablePrefetch && response.next_cursor) {
        if (prefetchTimeoutRef.current) {
          clearTimeout(prefetchTimeoutRef.current);
        }
        prefetchTimeoutRef.current = setTimeout(() => {
          prefetchNextPage(response.next_cursor!);
        }, prefetchDelay);
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        // Treat 404 as an empty dataset rather than a hard error so the UI can
        // render the empty state (important when users have no feeds yet).
        setData([]);
        setCursor(null);
        setHasMore(false);
        setError(null);
      } else {
        const error = err instanceof Error ? err : new Error("Failed to load data");
        setError(error);
        setData([]);
        setHasMore(false);
      }
    } finally {
      setIsLoading(false);
      setIsInitialLoading(false);
    }
  }, [fetchFn, limit, enablePrefetch, prefetchNextPage, prefetchDelay]);

  const loadMore = useCallback(async () => {
    if (isLoading || !hasMore || !cursor) {
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      let response: { data: T[]; next_cursor: string | null } | undefined;

      // Check if we have prefetched data
      if (enablePrefetch && prefetchCacheRef.current.has(cursor)) {
        const cachedResponse = prefetchCacheRef.current.get(cursor);
        if (cachedResponse !== "loading") {
          response = cachedResponse as {
            data: T[];
            next_cursor: string | null;
          };
          prefetchCacheRef.current.delete(cursor); // Use and remove from cache
        }
      }

      // If no cached data, fetch normally
      if (!response) {
        response = await fetchFn(cursor, limit);
      }

      setData((prevData) => [...prevData, ...response.data]);
      setCursor(response.next_cursor);
      setHasMore(response.next_cursor !== null);

      // Prefetch next page in background if available
      if (enablePrefetch && response.next_cursor) {
        if (prefetchTimeoutRef.current) {
          clearTimeout(prefetchTimeoutRef.current);
        }
        prefetchTimeoutRef.current = setTimeout(() => {
          prefetchNextPage(response.next_cursor!);
        }, prefetchDelay);
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        // No further pages available – clear pagination state but avoid showing
        // an error banner.
        setHasMore(false);
        setCursor(null);
        setError(null);
      } else {
        const error = err instanceof Error ? err : new Error("Failed to load more data");
        setError(error);
      }
    } finally {
      setIsLoading(false);
    }
  }, [fetchFn, cursor, limit, isLoading, hasMore, enablePrefetch, prefetchNextPage, prefetchDelay]);

  const refresh = useCallback(async () => {
    // Clear prefetch cache on refresh
    if (enablePrefetch) {
      prefetchCacheRef.current.clear();
      if (prefetchTimeoutRef.current) {
        clearTimeout(prefetchTimeoutRef.current);
      }
    }

    setCursor(null);
    setHasMore(true);
    await loadInitial();
  }, [loadInitial, enablePrefetch]);

  const reset = useCallback(() => {
    // Clear prefetch cache on reset
    if (enablePrefetch) {
      prefetchCacheRef.current.clear();
      if (prefetchTimeoutRef.current) {
        clearTimeout(prefetchTimeoutRef.current);
      }
    }

    setData([]);
    setCursor(null);
    setHasMore(true);
    setIsLoading(false);
    setError(null);
    setIsInitialLoading(true);
  }, [enablePrefetch]);

  // Auto-load initial data
  useEffect(() => {
    if (autoLoad) {
      loadInitial();
    }
  }, [autoLoad, loadInitial]);

  // Cleanup on unmount
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
    data,
    cursor,
    hasMore,
    isLoading,
    error,
    isInitialLoading,
    loadInitial,
    loadMore,
    refresh,
    reset,
  };
}

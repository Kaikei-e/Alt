"use client";

import { useState, useEffect, useCallback, useRef } from 'react';
import { Feed } from '@/schema/feed';
import { feedsApi } from '@/lib/api';

export interface UseReadFeedsResult {
  feeds: Feed[];
  isLoading: boolean;
  error: Error | null;
  hasMore: boolean;
  loadMore: () => void;
  refresh: () => void;
}

export const useReadFeeds = (
  initialLimit: number = 20
): UseReadFeedsResult => {
  const enablePrefetch = true;

  const [feeds, setFeeds] = useState<Feed[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [hasMore, setHasMore] = useState(true);
  const [cursor, setCursor] = useState<string | undefined>(undefined);

  // Prefetch cache for next cursor
  const prefetchCacheRef = useRef<Map<string, unknown>>(new Map());
  const prefetchTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  // Background prefetch function
  const prefetchNextPage = useCallback(async (nextCursor: string) => {
    if (!enablePrefetch) return; // Skip prefetch if disabled

    if (prefetchCacheRef.current.has(nextCursor)) {
      return; // Already prefetching or cached
    }

    try {
      // Mark as being prefetched
      prefetchCacheRef.current.set(nextCursor, 'loading');

      const response = await feedsApi.getReadFeedsWithCursor(nextCursor, initialLimit);

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
  }, [initialLimit, enablePrefetch]);

  const loadFeeds = useCallback(async (resetData: boolean = false) => {
    try {
      setIsLoading(true);
      setError(null);

      const currentCursor = resetData ? undefined : cursor;
      let response: { data: Feed[]; next_cursor: string | null } | undefined;

      // Check if we have prefetched data (only if prefetch is enabled)
      if (enablePrefetch && currentCursor && prefetchCacheRef.current.has(currentCursor)) {
        const cachedResponse = prefetchCacheRef.current.get(currentCursor);
        if (cachedResponse !== 'loading') {
          response = cachedResponse as { data: Feed[]; next_cursor: string | null };
          prefetchCacheRef.current.delete(currentCursor); // Use and remove from cache
        }
      }

      // If no cached data, fetch normally
      if (!response) {
        response = await feedsApi.getReadFeedsWithCursor(currentCursor, initialLimit);
      }

      if (resetData) {
        setFeeds(response.data);
      } else {
        setFeeds(prevFeeds => [...prevFeeds, ...response.data]);
      }

      setCursor(response.next_cursor || undefined);
      setHasMore(response.next_cursor !== null);

      // Prefetch next page in background if available (only if prefetch is enabled)
      if (enablePrefetch && response.next_cursor) {
        // Delay prefetch to avoid overwhelming the network
        if (prefetchTimeoutRef.current) {
          clearTimeout(prefetchTimeoutRef.current);
        }
        prefetchTimeoutRef.current = setTimeout(() => {
          prefetchNextPage(response.next_cursor!);
        }, 500); // 500ms delay
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
  }, [cursor, initialLimit, prefetchNextPage, enablePrefetch]);

  const loadMore = useCallback(() => {
    if (!isLoading && hasMore && cursor) {
      loadFeeds(false);
    }
  }, [isLoading, hasMore, cursor, loadFeeds]);

  const refresh = useCallback(async () => {
    // Clear prefetch cache on refresh
    prefetchCacheRef.current.clear();
    if (prefetchTimeoutRef.current) {
      clearTimeout(prefetchTimeoutRef.current);
    }

    setCursor(undefined);
    setHasMore(true);
    await loadFeeds(true);
  }, [loadFeeds]);

  // Load initial data
  useEffect(() => {
    const initialLoad = async () => {
      try {
        setIsLoading(true);
        setError(null);

        const response = await feedsApi.getReadFeedsWithCursor(undefined, initialLimit);
        setFeeds(response.data);
        setCursor(response.next_cursor || undefined);
        setHasMore(response.next_cursor !== null);

        // Prefetch next page in background if available (only if prefetch is enabled)
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
    feeds,
    isLoading,
    error,
    hasMore,
    loadMore,
    refresh,
  };
};
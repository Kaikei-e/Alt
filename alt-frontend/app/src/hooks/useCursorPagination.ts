"use client";

import { useState, useCallback } from "react";

export interface UseCursorPaginationOptions {
  limit?: number;
}

export interface UseCursorPaginationResult<T> {
  data: T[];
  cursor: string | null;
  hasMore: boolean;
  isLoading: boolean;
  error: string | null;
  isInitialLoading: boolean;
  loadInitial: () => Promise<void>;
  loadMore: () => Promise<void>;
  refresh: () => Promise<void>;
  reset: () => void;
}

export function useCursorPagination<T>(
  fetchFn: (
    cursor?: string,
    limit?: number,
  ) => Promise<{ data: T[]; next_cursor: string | null }>,
  options: UseCursorPaginationOptions = {},
): UseCursorPaginationResult<T> {
  const { limit = 20 } = options;

  const [data, setData] = useState<T[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(true);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isInitialLoading, setIsInitialLoading] = useState(true);

  const loadInitial = useCallback(async () => {
    setIsInitialLoading(true);
    setIsLoading(true);
    setError(null);

    try {
      const response = await fetchFn(undefined, limit);
      setData(response.data);
      setCursor(response.next_cursor);
      setHasMore(response.next_cursor !== null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load data");
      setData([]);
      setHasMore(false);
    } finally {
      setIsLoading(false);
      setIsInitialLoading(false);
    }
  }, [fetchFn, limit]);

  const loadMore = useCallback(async () => {
    if (isLoading || !hasMore || !cursor) {
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      const response = await fetchFn(cursor, limit);
      setData((prevData) => [...prevData, ...response.data]);
      setCursor(response.next_cursor);
      setHasMore(response.next_cursor !== null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load more data");
    } finally {
      setIsLoading(false);
    }
  }, [fetchFn, cursor, limit, isLoading, hasMore]);

  const refresh = useCallback(async () => {
    setCursor(null);
    setHasMore(true);
    await loadInitial();
  }, [loadInitial]);

  const reset = useCallback(() => {
    setData([]);
    setCursor(null);
    setHasMore(true);
    setIsLoading(false);
    setError(null);
    setIsInitialLoading(true);
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

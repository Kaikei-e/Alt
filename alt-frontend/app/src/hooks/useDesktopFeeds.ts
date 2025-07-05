import { useState, useEffect, useCallback } from 'react';
import { Feed } from '@/schema/feed';
import { feedsApi } from '@/lib/api';

export const useDesktopFeeds = () => {
  const [feeds, setFeeds] = useState<Feed[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [hasMore, setHasMore] = useState(true);
  const [cursor, setCursor] = useState<string | undefined>(undefined);



  const fetchNextPage = useCallback(async () => {
    if (!hasMore || isLoading) {
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      const result = await feedsApi.getFeedsWithCursor(cursor, 20);

      if (!result) {
        throw new Error('No data received from API');
      }

      setFeeds(prev => [...prev, ...(result.data || [])]);
      setCursor(result.next_cursor || undefined);
      setHasMore(result.next_cursor !== null);
    } catch (err) {
      setError(err as Error);
      setHasMore(false);
    } finally {
      setIsLoading(false);
    }
  }, [hasMore, isLoading, cursor]);

  const markAsRead = useCallback(async (feedId: string) => {
    try {
      await feedsApi.updateFeedReadStatus(feedId);
      // Note: Feed型にはisReadプロパティがないため、フィードリストから削除する代わりに
      // ここでは何もしない。実際の読み取り状態管理は別の仕組みで行う
    } catch (err) {
      console.error('Failed to mark as read:', err);
    }
  }, []);

  const toggleFavorite = useCallback(async (feedId: string) => {
    try {
      // Feed型にはisFavoritedプロパティがないため、単純にAPIを呼び出すのみ
      await feedsApi.toggleFavorite(feedId, true);
    } catch (err) {
      console.error('Failed to toggle favorite:', err);
    }
  }, []);

  const toggleBookmark = useCallback(async (feedId: string) => {
    try {
      // Feed型にはisBookmarkedプロパティがないため、単純にAPIを呼び出すのみ
      await feedsApi.toggleBookmark(feedId, true);
    } catch (err) {
      console.error('Failed to toggle bookmark:', err);
    }
  }, []);

  // Preload next page for better performance
  const preloadNextPage = useCallback(async () => {
    if (cursor && hasMore) {
      try {
        // Preload in background without affecting UI state
        await feedsApi.getFeedsWithCursor(cursor, 20);
      } catch (err) {
        // Silently fail preloading to not affect main functionality
        console.warn('Failed to preload next page:', err);
      }
    }
  }, [cursor, hasMore]);

  useEffect(() => {
    // Initial fetch on mount
    const initialFetch = async () => {
      setIsLoading(true);
      setError(null);

      try {
        const result = await feedsApi.getFeedsWithCursor(undefined, 20);

        if (!result) {
          throw new Error('No data received from API');
        }

        setFeeds(result.data || []);
        setCursor(result.next_cursor || undefined);
        setHasMore(result.next_cursor !== null);

        // Preload next page after initial load for better UX
        if (result.next_cursor) {
          setTimeout(() => {
            feedsApi.getFeedsWithCursor(result.next_cursor || undefined, 20).catch(() => {});
          }, 1000); // Preload after 1 second
        }
      } catch (err) {
        setError(err as Error);
        setHasMore(false);
        setFeeds([]);
      } finally {
        setIsLoading(false);
      }
    };

    initialFetch();
  }, []); // Only run on mount

  // Preload next page when getting close to the current page end
  useEffect(() => {
    if (feeds.length > 0 && feeds.length % 15 === 0) { // Every 15 items
      preloadNextPage();
    }
  }, [feeds.length, preloadNextPage]);

  return {
    feeds,
    isLoading,
    error,
    hasMore,
    fetchNextPage,
    markAsRead,
    toggleFavorite,
    toggleBookmark,
    refresh: useCallback(async () => {
      setIsLoading(true);
      setError(null);
      setCursor(undefined);
      setHasMore(true);

      try {
        const result = await feedsApi.getFeedsWithCursor(undefined, 20);

        if (!result) {
          throw new Error('No data received from API');
        }

        setFeeds(result.data || []);
        setCursor(result.next_cursor || undefined);
        setHasMore(result.next_cursor !== null);
      } catch (err) {
        setError(err as Error);
        setHasMore(false);
        setFeeds([]);
      } finally {
        setIsLoading(false);
      }
    }, [])
  };
};
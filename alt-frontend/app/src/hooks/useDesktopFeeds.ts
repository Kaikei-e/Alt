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
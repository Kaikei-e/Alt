import { useState, useEffect, useCallback } from 'react';
import { DesktopFeed } from '@/types/desktop-feed';
import { feedsApi } from '@/lib/api';

export const useDesktopFeeds = () => {
  const [feeds, setFeeds] = useState<DesktopFeed[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [hasMore, setHasMore] = useState(true);
  const [cursor, setCursor] = useState<string | null>(null);



  const fetchNextPage = useCallback(async () => {
    if (!hasMore || isLoading || !cursor) {
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      const result = await feedsApi.getDesktopFeeds(cursor);

      if (!result) {
        throw new Error('No data received from API');
      }

      setFeeds(prev => [...prev, ...(result.feeds || [])]);
      setCursor(result.nextCursor || null);
      setHasMore(result.hasMore || false);
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
      setFeeds(prev => prev.map(feed =>
        feed.id === feedId ? { ...feed, isRead: true } : feed
      ));
    } catch (err) {
      console.error('Failed to mark as read:', err);
    }
  }, []);

  const toggleFavorite = useCallback(async (feedId: string) => {
    try {
      const feed = feeds.find(f => f.id === feedId);
      if (feed) {
        await feedsApi.toggleFavorite(feedId, !feed.isFavorited);
        setFeeds(prev => prev.map(f =>
          f.id === feedId ? { ...f, isFavorited: !f.isFavorited } : f
        ));
      }
    } catch (err) {
      console.error('Failed to toggle favorite:', err);
    }
  }, [feeds]);

  const toggleBookmark = useCallback(async (feedId: string) => {
    try {
      const feed = feeds.find(f => f.id === feedId);
      if (feed) {
        await feedsApi.toggleBookmark(feedId, !feed.isBookmarked);
        setFeeds(prev => prev.map(f =>
          f.id === feedId ? { ...f, isBookmarked: !f.isBookmarked } : f
        ));
      }
    } catch (err) {
      console.error('Failed to toggle bookmark:', err);
    }
  }, [feeds]);

  useEffect(() => {
    // Initial fetch on mount
    const initialFetch = async () => {
      setIsLoading(true);
      setError(null);

      try {
        const result = await feedsApi.getDesktopFeeds(null);

        if (!result) {
          throw new Error('No data received from API');
        }

        setFeeds(result.feeds || []);
        setCursor(result.nextCursor || null);
        setHasMore(result.hasMore || false);
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
      setCursor(null);
      setHasMore(true);

      try {
        const result = await feedsApi.getDesktopFeeds(null);

        if (!result) {
          throw new Error('No data received from API');
        }

        setFeeds(result.feeds || []);
        setCursor(result.nextCursor || null);
        setHasMore(result.hasMore || false);
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
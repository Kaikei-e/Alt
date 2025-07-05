import { useState, useEffect, useCallback } from 'react';
import { DesktopFeed } from '@/types/desktop-feed';
import { feedsApi } from '@/lib/api';

export const useDesktopFeeds = () => {
  const [feeds, setFeeds] = useState<DesktopFeed[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [hasMore, setHasMore] = useState(true);
  const [cursor, setCursor] = useState<string | null>(null);

  const fetchFeeds = useCallback(async (reset = false) => {
    setIsLoading(true);
    setError(null);

    try {
      const result = await feedsApi.getDesktopFeeds(reset ? null : cursor);
      
      setFeeds(prev => reset ? result.feeds : [...prev, ...result.feeds]);
      setCursor(result.nextCursor);
      setHasMore(result.hasMore);
    } catch (err) {
      setError(err as Error);
    } finally {
      setIsLoading(false);
    }
  }, [cursor]);

  const fetchNextPage = useCallback(() => {
    if (hasMore && !isLoading) {
      fetchFeeds(false);
    }
  }, [hasMore, isLoading, fetchFeeds]);

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
    fetchFeeds(true);
  }, [fetchFeeds]);

  return {
    feeds,
    isLoading,
    error,
    hasMore,
    fetchNextPage,
    markAsRead,
    toggleFavorite,
    toggleBookmark,
    refresh: () => fetchFeeds(true)
  };
};
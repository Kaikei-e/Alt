import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useDesktopFeeds } from './useDesktopFeeds';
import { feedsApi } from '@/lib/api';
import { mockDesktopFeeds } from '@/data/mockDesktopFeeds';

// Mock the API
vi.mock('@/lib/api', () => ({
  feedsApi: {
    getDesktopFeeds: vi.fn(),
    updateFeedReadStatus: vi.fn(),
    toggleFavorite: vi.fn(),
    toggleBookmark: vi.fn(),
  },
}));

describe('useDesktopFeeds', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    
    // Default mock implementation
    vi.mocked(feedsApi.getDesktopFeeds).mockResolvedValue({
      feeds: mockDesktopFeeds,
      nextCursor: null,
      hasMore: false,
      totalCount: mockDesktopFeeds.length
    });
    
    vi.mocked(feedsApi.updateFeedReadStatus).mockResolvedValue({
      message: 'Feed marked as read'
    });
    
    vi.mocked(feedsApi.toggleFavorite).mockResolvedValue({
      message: 'Favorite toggled'
    });
    
    vi.mocked(feedsApi.toggleBookmark).mockResolvedValue({
      message: 'Bookmark toggled'
    });
  });

  it('should fetch feeds on mount', async () => {
    const { result } = renderHook(() => useDesktopFeeds());

    expect(result.current.isLoading).toBe(true);

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(feedsApi.getDesktopFeeds).toHaveBeenCalledWith(null);
    expect(result.current.feeds).toEqual(mockDesktopFeeds);
    expect(result.current.error).toBeNull();
  });

  it('should handle mark as read action', async () => {
    const { result } = renderHook(() => useDesktopFeeds());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    await result.current.markAsRead('1');

    expect(feedsApi.updateFeedReadStatus).toHaveBeenCalledWith('1');
    
    // Wait for state update
    await waitFor(() => {
      const updatedFeed = result.current.feeds.find(f => f.id === '1');
      expect(updatedFeed?.isRead).toBe(true);
    });
  });

  it('should handle toggle favorite action', async () => {
    const { result } = renderHook(() => useDesktopFeeds());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    const feedId = '1';
    const originalFavoriteStatus = result.current.feeds.find(f => f.id === feedId)?.isFavorited;

    await result.current.toggleFavorite(feedId);

    expect(feedsApi.toggleFavorite).toHaveBeenCalledWith(feedId, !originalFavoriteStatus);
    
    // Wait for state update
    await waitFor(() => {
      const updatedFeed = result.current.feeds.find(f => f.id === feedId);
      expect(updatedFeed?.isFavorited).toBe(!originalFavoriteStatus);
    });
  });

  it('should handle toggle bookmark action', async () => {
    const { result } = renderHook(() => useDesktopFeeds());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    const feedId = '1';
    const originalBookmarkStatus = result.current.feeds.find(f => f.id === feedId)?.isBookmarked;

    await result.current.toggleBookmark(feedId);

    expect(feedsApi.toggleBookmark).toHaveBeenCalledWith(feedId, !originalBookmarkStatus);
    
    // Wait for state update
    await waitFor(() => {
      const updatedFeed = result.current.feeds.find(f => f.id === feedId);
      expect(updatedFeed?.isBookmarked).toBe(!originalBookmarkStatus);
    });
  });

  it('should handle fetch next page', async () => {
    vi.mocked(feedsApi.getDesktopFeeds).mockResolvedValueOnce({
      feeds: mockDesktopFeeds.slice(0, 3),
      nextCursor: 'cursor-1',
      hasMore: true,
      totalCount: mockDesktopFeeds.length
    });

    const { result } = renderHook(() => useDesktopFeeds());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.hasMore).toBe(true);

    // Mock next page response
    vi.mocked(feedsApi.getDesktopFeeds).mockResolvedValueOnce({
      feeds: mockDesktopFeeds.slice(3),
      nextCursor: null,
      hasMore: false,
      totalCount: mockDesktopFeeds.length
    });

    await result.current.fetchNextPage();

    await waitFor(() => {
      expect(feedsApi.getDesktopFeeds).toHaveBeenCalledWith('cursor-1');
      expect(result.current.feeds).toHaveLength(mockDesktopFeeds.length);
      expect(result.current.hasMore).toBe(false);
    });
  });

  it('should handle API errors gracefully', async () => {
    const errorMessage = 'Failed to fetch feeds';
    vi.mocked(feedsApi.getDesktopFeeds).mockRejectedValue(new Error(errorMessage));

    const { result } = renderHook(() => useDesktopFeeds());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.error).toEqual(new Error(errorMessage));
    expect(result.current.feeds).toEqual([]);
  });

  it('should refresh feeds', async () => {
    const { result } = renderHook(() => useDesktopFeeds());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Clear previous calls
    vi.clearAllMocks();

    await result.current.refresh();

    expect(feedsApi.getDesktopFeeds).toHaveBeenCalledWith(null);
  });
});
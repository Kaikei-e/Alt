import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useDesktopFeeds } from './useDesktopFeeds';
import { feedsApi } from '@/lib/api';

// Mock the API
vi.mock('@/lib/api', () => ({
  feedsApi: {
    getFeedsWithCursor: vi.fn(),
    updateFeedReadStatus: vi.fn(),
    toggleFavorite: vi.fn(),
    toggleBookmark: vi.fn(),
  },
}));

describe('useDesktopFeeds', () => {
  beforeEach(() => {
    vi.clearAllMocks();

    // Default mock implementation for getFeedsWithCursor
    vi.mocked(feedsApi.getFeedsWithCursor).mockResolvedValue({
      data: [
        {
          id: '1',
          title: 'Test Feed 1',
          description: 'Test Description 1',
          link: 'https://example.com/1',
          published: '2024-01-01T00:00:00Z'
        }
      ],
      next_cursor: null
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

    expect(feedsApi.getFeedsWithCursor).toHaveBeenCalledWith(undefined, 20);
    expect(result.current.feeds).toHaveLength(1);
    expect(result.current.feeds[0]).toEqual(
      expect.objectContaining({
        id: '1',
        title: 'Test Feed 1',
        description: 'Test Description 1',
        link: 'https://example.com/1'
      })
    );
    expect(result.current.error).toBeNull();
  });

  it('should handle mark as read action', async () => {
    const { result } = renderHook(() => useDesktopFeeds());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    await result.current.markAsRead('1');

    expect(feedsApi.updateFeedReadStatus).toHaveBeenCalledWith('1');
  });

  it('should handle toggle favorite action', async () => {
    const { result } = renderHook(() => useDesktopFeeds());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    const feedId = '1';

    await result.current.toggleFavorite(feedId);

    expect(feedsApi.toggleFavorite).toHaveBeenCalledWith(feedId, true);
  });

  it('should handle toggle bookmark action', async () => {
    const { result } = renderHook(() => useDesktopFeeds());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    const feedId = '1';

    await result.current.toggleBookmark(feedId);

    expect(feedsApi.toggleBookmark).toHaveBeenCalledWith(feedId, true);
  });

  it('should handle fetch next page', async () => {
    // Clear all mocks
    vi.clearAllMocks();

    // Mock consecutive calls - first call (initial fetch), second call (next page)
    const mockApiCall = vi.mocked(feedsApi.getFeedsWithCursor);

    mockApiCall
      .mockResolvedValueOnce({
        data: [
          {
            id: '1',
            title: 'Test Feed 1',
            description: 'Test Description 1',
            link: 'https://example.com/1',
            published: '2024-01-01T00:00:00Z'
          }
        ],
        next_cursor: 'cursor-1'
      })
      .mockResolvedValueOnce({
        data: [
          {
            id: '2',
            title: 'Test Feed 2',
            description: 'Test Description 2',
            link: 'https://example.com/2',
            published: '2024-01-02T00:00:00Z'
          }
        ],
        next_cursor: null
      });

    // Setup other mocks
    vi.mocked(feedsApi.updateFeedReadStatus).mockResolvedValue({
      message: 'Feed marked as read'
    });

    vi.mocked(feedsApi.toggleFavorite).mockResolvedValue({
      message: 'Favorite toggled'
    });

    vi.mocked(feedsApi.toggleBookmark).mockResolvedValue({
      message: 'Bookmark toggled'
    });

    const { result } = renderHook(() => useDesktopFeeds());

    // Wait for the initial fetch to complete
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
      expect(result.current.error).toBeNull();
    }, { timeout: 3000 });

    // Verify initial state
    expect(result.current.hasMore).toBe(true);
    expect(result.current.feeds).toHaveLength(1);
    expect(mockApiCall).toHaveBeenCalledTimes(1);
    expect(mockApiCall).toHaveBeenCalledWith(undefined, 20);

    // Fetch next page
    await result.current.fetchNextPage();

    // Wait for the second fetch to complete
    await waitFor(() => {
      expect(result.current.feeds).toHaveLength(2);
      expect(result.current.hasMore).toBe(false);
    }, { timeout: 3000 });

    expect(mockApiCall).toHaveBeenCalledTimes(2);
    expect(mockApiCall).toHaveBeenLastCalledWith('cursor-1', 20);
  });

  it('should handle API errors gracefully', async () => {
    const errorMessage = 'Failed to fetch feeds';
    vi.mocked(feedsApi.getFeedsWithCursor).mockRejectedValue(new Error(errorMessage));

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

    expect(feedsApi.getFeedsWithCursor).toHaveBeenCalledWith(undefined, 20);
  });
});
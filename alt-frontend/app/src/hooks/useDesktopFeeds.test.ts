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
});
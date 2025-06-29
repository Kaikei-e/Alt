import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { useReadFeeds } from '@/hooks/useReadFeeds';
import { feedsApi } from '@/lib/api';
import { Feed } from '@/schema/feed';

// Mock the feedsApi
vi.mock('@/lib/api', () => ({
  feedsApi: {
    getReadFeedsWithCursor: vi.fn(),
  },
}));

describe('useReadFeeds Hook - TDD Implementation', () => {
  const mockFeeds: Feed[] = [
    {
      id: '1',
      title: 'Test Feed 1',
      description: 'Description 1',
      link: 'https://example.com/feed1',
      published: '2024-01-01T00:00:00Z',
    },
    {
      id: '2',
      title: 'Test Feed 2',
      description: 'Description 2',
      link: 'https://example.com/feed2',
      published: '2024-01-02T00:00:00Z',
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('initial state', () => {
    it('should initialize with correct default values', async () => {
      const mockGetReadFeeds = vi.mocked(feedsApi.getReadFeedsWithCursor);
      mockGetReadFeeds.mockResolvedValue({
        data: mockFeeds,
        next_cursor: 'cursor123',
      });

      const { result } = renderHook(() => useReadFeeds());

      expect(result.current.feeds).toEqual([]);
      expect(result.current.isLoading).toBe(true);
      expect(result.current.error).toBeNull();
      expect(result.current.hasMore).toBe(true);
      expect(typeof result.current.loadMore).toBe('function');
      expect(typeof result.current.refresh).toBe('function');

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.feeds).toEqual(mockFeeds);
      expect(result.current.hasMore).toBe(true);
    });

    it('should initialize with custom limit', async () => {
      const mockGetReadFeeds = vi.mocked(feedsApi.getReadFeedsWithCursor);
      mockGetReadFeeds.mockResolvedValue({
        data: mockFeeds,
        next_cursor: null,
      });

      renderHook(() => useReadFeeds(50));

      await waitFor(() => {
        expect(mockGetReadFeeds).toHaveBeenCalledWith(undefined, 50);
      });
    });
  });

  describe('data loading', () => {
    it('should load initial data automatically', async () => {
      const mockGetReadFeeds = vi.mocked(feedsApi.getReadFeedsWithCursor);
      mockGetReadFeeds.mockResolvedValue({
        data: mockFeeds,
        next_cursor: 'cursor123',
      });

      const { result } = renderHook(() => useReadFeeds());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(mockGetReadFeeds).toHaveBeenCalledWith(undefined, 20);
      expect(result.current.feeds).toEqual(mockFeeds);
      expect(result.current.hasMore).toBe(true);
    });

    it('should handle loading more data', async () => {
      const mockGetReadFeeds = vi.mocked(feedsApi.getReadFeedsWithCursor);
      mockGetReadFeeds
        .mockResolvedValueOnce({
          data: mockFeeds,
          next_cursor: 'cursor123',
        })
        .mockResolvedValueOnce({
          data: [mockFeeds[0]],
          next_cursor: null,
        });

      const { result } = renderHook(() => useReadFeeds());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      await act(async () => {
        result.current.loadMore();
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(mockGetReadFeeds).toHaveBeenCalledTimes(2);
      expect(mockGetReadFeeds).toHaveBeenNthCalledWith(2, 'cursor123', 20);
      expect(result.current.feeds).toHaveLength(3);
      expect(result.current.hasMore).toBe(false);
    });
  });

  describe('refresh functionality', () => {
    it('should refresh data', async () => {
      const mockGetReadFeeds = vi.mocked(feedsApi.getReadFeedsWithCursor);
      mockGetReadFeeds.mockResolvedValue({
        data: mockFeeds,
        next_cursor: null,
      });

      const { result } = renderHook(() => useReadFeeds());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      await act(async () => {
        result.current.refresh();
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(mockGetReadFeeds).toHaveBeenCalledTimes(2);
      expect(result.current.feeds).toEqual(mockFeeds);
    });
  });

  describe('error handling', () => {
    it('should handle API errors', async () => {
      const mockGetReadFeeds = vi.mocked(feedsApi.getReadFeedsWithCursor);
      const error = new Error('API Error');
      mockGetReadFeeds.mockRejectedValue(error);

      const { result } = renderHook(() => useReadFeeds());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.error).toEqual(error);
      expect(result.current.feeds).toEqual([]);
      expect(result.current.hasMore).toBe(false);
    });
  });

  describe('loading states', () => {
    it('should manage loading states correctly', async () => {
      const mockGetReadFeeds = vi.mocked(feedsApi.getReadFeedsWithCursor);
      let resolvePromise: (value: any) => void;
      const promise = new Promise<any>((resolve) => {
        resolvePromise = resolve;
      });
      mockGetReadFeeds.mockReturnValue(promise);

      const { result } = renderHook(() => useReadFeeds());

      expect(result.current.isLoading).toBe(true);

      resolvePromise!({
        data: mockFeeds,
        next_cursor: null,
      });

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.feeds).toEqual(mockFeeds);
    });
  });
});
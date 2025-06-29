import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useReadFeeds } from '@/hooks/useReadFeeds';
import { feedsApi } from '@/lib/api';

// Mock the API
vi.mock('@/lib/api', () => ({
  feedsApi: {
    getReadFeedsWithCursor: vi.fn(),
  },
}));

const mockFeedsApi = feedsApi as any;

describe('useReadFeeds Performance Tests - PROTECTED', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  const generateMockFeeds = (count: number, offset: number = 0) => {
    return Array.from({ length: count }, (_, index) => ({
      id: `feed-${offset + index}`,
      title: `Test Feed ${offset + index}`,
      description: `Description for test feed ${offset + index}`,
      link: `https://example.com/feed-${offset + index}`,
      pub_date: new Date(Date.now() - (offset + index) * 1000 * 60 * 60).toISOString(),
      read_status: true,
    }));
  };

  it('基本的なデータ読み込み確認 (PROTECTED)', async () => {
    const PAGE_SIZE = 20;

    mockFeedsApi.getReadFeedsWithCursor.mockResolvedValue({
      data: generateMockFeeds(PAGE_SIZE),
      next_cursor: null,
    });

    const { result } = renderHook(() => useReadFeeds(PAGE_SIZE));

    // Wait for initial load
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.feeds).toHaveLength(PAGE_SIZE);
    expect(result.current.hasMore).toBe(false);
    expect(result.current.error).toBeNull();

    // Verify API was called once
    expect(mockFeedsApi.getReadFeedsWithCursor).toHaveBeenCalledTimes(1);
  });

  it('API呼び出し最適化確認 (PROTECTED)', async () => {
    const PAGE_SIZE = 20;
    let callCount = 0;

    mockFeedsApi.getReadFeedsWithCursor.mockImplementation(async () => {
      callCount++;
      return {
        data: generateMockFeeds(PAGE_SIZE, (callCount - 1) * PAGE_SIZE),
        next_cursor: callCount < 2 ? 'next-cursor' : null,
      };
    });

    const { result } = renderHook(() => useReadFeeds(PAGE_SIZE));

    // Wait for initial load
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.feeds).toHaveLength(PAGE_SIZE);
    // Initial load should only call API once
    expect(callCount).toBe(1);

    // Single loadMore call
    result.current.loadMore();

    await waitFor(() => {
      expect(result.current.feeds).toHaveLength(40);
    });

    // Should have made exactly 2 API calls
    expect(callCount).toBe(2);
  });

  it('エラー処理の確認 (PROTECTED)', async () => {
    const PAGE_SIZE = 20;

    // Mock API to fail first, then succeed
    mockFeedsApi.getReadFeedsWithCursor
      .mockRejectedValueOnce(new Error('Network error'))
      .mockResolvedValueOnce({
        data: generateMockFeeds(PAGE_SIZE),
        next_cursor: null,
      });

    const { result } = renderHook(() => useReadFeeds(PAGE_SIZE));

    // Wait for error state
    await waitFor(() => {
      expect(result.current.error).toBeTruthy();
      expect(result.current.isLoading).toBe(false);
    });

    // Retry operation
    result.current.refresh();

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
      expect(result.current.error).toBeNull();
      expect(result.current.feeds).toHaveLength(PAGE_SIZE);
    });

    // Should have made exactly 2 API calls (failed + successful)
    expect(mockFeedsApi.getReadFeedsWithCursor).toHaveBeenCalledTimes(2);
  });

  it('先読み機能の動作確認 (PROTECTED)', async () => {
    const PAGE_SIZE = 20;
    let apiCallCount = 0;

    mockFeedsApi.getReadFeedsWithCursor.mockImplementation(async (cursor?: string) => {
      apiCallCount++;
      
      const offset = cursor === 'page-2' ? 20 : 0;
      return {
        data: generateMockFeeds(PAGE_SIZE, offset),
        next_cursor: cursor ? null : 'page-2',
      };
    });

    const { result } = renderHook(() => useReadFeeds(PAGE_SIZE));

    // Wait for initial load
    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.feeds).toHaveLength(PAGE_SIZE);
    expect(result.current.hasMore).toBe(true);

    // Wait for prefetch to potentially complete
    await new Promise(resolve => setTimeout(resolve, 600));

    // Load more (should use prefetched data if available)
    result.current.loadMore();

    await waitFor(() => {
      expect(result.current.feeds).toHaveLength(40);
    });

    // Verify prefetch behavior - should have made initial + possibly prefetch + loadMore calls
    expect(apiCallCount).toBeGreaterThanOrEqual(2);
    expect(apiCallCount).toBeLessThanOrEqual(3); // Allow for prefetch optimization
  });
});
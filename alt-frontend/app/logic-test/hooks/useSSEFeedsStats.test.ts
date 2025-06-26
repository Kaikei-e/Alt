import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useSSEFeedsStats } from "@/hooks/useSSEFeedsStats";

// Mock the SSE API module
vi.mock("@/lib/apiSse", () => ({
  setupSSEWithReconnect: vi.fn(),
}));

describe("useSSEFeedsStats Hook", () => {
  let mockSetupSSEWithReconnect: any;
  let mockEventSource: any;
  let mockCleanup: any;

  beforeEach(async () => {
    // Mock EventSource for Node.js environment
    global.EventSource = vi.fn(() => ({
      readyState: 0, // CONNECTING
      close: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })) as any;
    Object.assign(global.EventSource, {
      OPEN: 1,
      CLOSED: 2,
    });

    vi.clearAllMocks();
    vi.useFakeTimers();

    // Mock environment variable
    vi.stubEnv('NEXT_PUBLIC_API_BASE_URL', 'http://localhost:8080/api');

    // Mock cleanup function
    mockCleanup = vi.fn();

    // Mock EventSource instance
    mockEventSource = {
      readyState: 1, // OPEN
      close: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    };

    // Mock setupSSEWithReconnect
    mockSetupSSEWithReconnect = vi.fn(() => ({
      eventSource: mockEventSource,
      cleanup: mockCleanup,
    }));

    const { setupSSEWithReconnect } = await import("@/lib/apiSse");
    vi.mocked(setupSSEWithReconnect).mockImplementation(mockSetupSSEWithReconnect);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.unstubAllEnvs();
  });

  describe("Initial State and Connection", () => {
    it("should initialize with default values", () => {
      const { result } = renderHook(() => useSSEFeedsStats());

      expect(result.current.feedAmount).toBe(0);
      expect(result.current.unsummarizedArticlesAmount).toBe(0);
      expect(result.current.totalArticlesAmount).toBe(0);
      expect(result.current.isConnected).toBe(false);
      expect(result.current.retryCount).toBe(0);
      expect(result.current.progressResetTrigger).toBe(0);
      expect(typeof result.current.resetProgress).toBe("function");
    });

    it("should create SSE connection only once", () => {
      renderHook(() => useSSEFeedsStats());

      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);
      expect(mockSetupSSEWithReconnect).toHaveBeenCalledWith(
        "http://localhost:8080/api/v1/sse/feeds/stats",
        expect.any(Function), // onData callback
        expect.any(Function), // onError callback
        3, // maxReconnectAttempts
        expect.any(Function), // onOpen callback
      );
    });

    it("should not recreate SSE connection on re-renders", () => {
      const { rerender } = renderHook(() => useSSEFeedsStats());

      // Trigger multiple re-renders
      rerender();
      rerender();
      rerender();

      // SSE should still be created only once
      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);
    });
  });

  describe("Data Handling", () => {
    it("should update feed stats when receiving valid data", () => {
      const { result } = renderHook(() => useSSEFeedsStats());

      // Get the onData callback from the setupSSEWithReconnect call
      const onDataCallback = mockSetupSSEWithReconnect.mock.calls[0][1];

      const mockData = {
        feed_amount: { amount: 25 },
        unsummarized_feed: { amount: 18 },
        total_articles: { amount: 1337 },
      };

      act(() => {
        onDataCallback(mockData);
      });

      expect(result.current.feedAmount).toBe(25);
      expect(result.current.unsummarizedArticlesAmount).toBe(18);
      expect(result.current.totalArticlesAmount).toBe(1337);
      expect(result.current.progressResetTrigger).toBe(1);
    });

    it("should handle missing or invalid data gracefully", () => {
      const { result } = renderHook(() => useSSEFeedsStats());
      const onDataCallback = mockSetupSSEWithReconnect.mock.calls[0][1];

      // Test with missing fields
      act(() => {
        onDataCallback({
          feed_amount: { amount: 10 },
          // Missing unsummarized_feed and total_articles
        });
      });

      expect(result.current.feedAmount).toBe(10);
      expect(result.current.unsummarizedArticlesAmount).toBe(0);
      expect(result.current.totalArticlesAmount).toBe(0);

      // Test with invalid values
      act(() => {
        onDataCallback({
          feed_amount: { amount: NaN },
          unsummarized_feed: { amount: -5 },
          total_articles: { amount: Infinity },
        });
      });

      expect(result.current.feedAmount).toBe(0);
      expect(result.current.unsummarizedArticlesAmount).toBe(0);
      expect(result.current.totalArticlesAmount).toBe(0);
    });

    it("should trigger progress reset when receiving data", () => {
      const { result } = renderHook(() => useSSEFeedsStats());
      const onDataCallback = mockSetupSSEWithReconnect.mock.calls[0][1];

      const initialTrigger = result.current.progressResetTrigger;

      act(() => {
        onDataCallback({ feed_amount: { amount: 5 } });
      });

      expect(result.current.progressResetTrigger).toBe(initialTrigger + 1);

      act(() => {
        onDataCallback({ feed_amount: { amount: 10 } });
      });

      expect(result.current.progressResetTrigger).toBe(initialTrigger + 2);
    });
  });

  describe("Connection State Management", () => {
    it("should update connection state when SSE opens", () => {
      const { result } = renderHook(() => useSSEFeedsStats());
      const onOpenCallback = mockSetupSSEWithReconnect.mock.calls[0][4];

      expect(result.current.isConnected).toBe(false);

      act(() => {
        onOpenCallback();
      });

      expect(result.current.isConnected).toBe(true);
      expect(result.current.retryCount).toBe(0);
    });

    it("should handle connection errors and update retry count", () => {
      const { result } = renderHook(() => useSSEFeedsStats());
      const onErrorCallback = mockSetupSSEWithReconnect.mock.calls[0][2];

      act(() => {
        onErrorCallback();
      });

      expect(result.current.isConnected).toBe(false);
      expect(result.current.retryCount).toBe(1);

      act(() => {
        onErrorCallback();
      });

      expect(result.current.retryCount).toBe(2);
    });

    it("should reset retry count when connection is successful", () => {
      const { result } = renderHook(() => useSSEFeedsStats());
      const onErrorCallback = mockSetupSSEWithReconnect.mock.calls[0][2];
      const onDataCallback = mockSetupSSEWithReconnect.mock.calls[0][1];

      // Simulate errors
      act(() => {
        onErrorCallback();
        onErrorCallback();
      });

      expect(result.current.retryCount).toBe(2);

      // Simulate successful data reception
      act(() => {
        onDataCallback({ feed_amount: { amount: 1 } });
      });

      expect(result.current.retryCount).toBe(0);
      expect(result.current.isConnected).toBe(true);
    });
  });

  describe("Health Check", () => {
    it("should perform health checks at regular intervals", () => {
      renderHook(() => useSSEFeedsStats());

      // Fast-forward time to trigger health check
      act(() => {
        vi.advanceTimersByTime(5000);
      });

      // Health check should have run (testing internal timer)
      expect(vi.getTimerCount()).toBeGreaterThan(0);
    });

    it("should update connection status based on data reception timing", () => {
      const { result } = renderHook(() => useSSEFeedsStats());
      const onDataCallback = mockSetupSSEWithReconnect.mock.calls[0][1];

      // Set connection as open
      mockEventSource.readyState = 1; // OPEN

      // Receive data recently
      act(() => {
        onDataCallback({ feed_amount: { amount: 1 } });
      });

      expect(result.current.isConnected).toBe(true);

      // Fast-forward time beyond timeout (15s)
      act(() => {
        vi.advanceTimersByTime(20000);
      });

      // Health check should mark as disconnected due to no recent data
      expect(result.current.isConnected).toBe(false);
    });
  });

  describe("Manual Progress Reset", () => {
    it("should allow manual progress reset", () => {
      const { result } = renderHook(() => useSSEFeedsStats());

      const initialTrigger = result.current.progressResetTrigger;

      act(() => {
        result.current.resetProgress();
      });

      expect(result.current.progressResetTrigger).toBe(initialTrigger + 1);

      act(() => {
        result.current.resetProgress();
      });

      expect(result.current.progressResetTrigger).toBe(initialTrigger + 2);
    });
  });

  describe("Cleanup", () => {
    it("should cleanup SSE connection on unmount", () => {
      const { unmount } = renderHook(() => useSSEFeedsStats());

      unmount();

      expect(mockCleanup).toHaveBeenCalledTimes(1);
    });

    it("should not update state after unmount", () => {
      const { result, unmount } = renderHook(() => useSSEFeedsStats());
      const onDataCallback = mockSetupSSEWithReconnect.mock.calls[0][1];

      unmount();

      // Try to update state after unmount (should be ignored)
      act(() => {
        onDataCallback({ feed_amount: { amount: 999 } });
      });

      // State should not have changed
      expect(result.current.feedAmount).toBe(0);
    });
  });

  describe("Stability - NO Infinite Reconnections", () => {
    it("should maintain stable SSE connection across multiple interactions", () => {
      const { result } = renderHook(() => useSSEFeedsStats());
      const onDataCallback = mockSetupSSEWithReconnect.mock.calls[0][1];

      // Simulate multiple data receptions and manual resets
      for (let i = 0; i < 10; i++) {
        act(() => {
          onDataCallback({ feed_amount: { amount: i } });
          result.current.resetProgress();
        });
      }

      // SSE should still be created only once
      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);
      expect(result.current.feedAmount).toBe(9);
      expect(result.current.progressResetTrigger).toBe(20); // 10 from data + 10 from manual
    });
  });
});
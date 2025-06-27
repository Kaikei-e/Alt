import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, act } from "@testing-library/react";
import React from "react";
import FeedsStatsPage from "@/app/mobile/feeds/stats/page";

vi.mock("@chakra-ui/react", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@chakra-ui/react")>();

  const chakraMock = (Component: string) => {
    return ({ children, ...props }: any) => {
      const validProps: { [key: string]: any } = {};
      for (const key in props) {
        if (key === "onClick" || key.startsWith("data-")) {
          validProps[key] = props[key];
        }
      }
      return React.createElement(Component, validProps, children);
    };
  };

  return {
    ...actual,
    Box: chakraMock("div"),
    Flex: chakraMock("div"),
    Text: chakraMock("span"),
  };
});

// Mock the SSE API module
vi.mock("@/lib/apiSse", () => ({
  setupSSEWithReconnect: vi.fn(),
}));

// Mock other dependencies
vi.mock("@/components/mobile/utils/FloatingMenu", () => ({
  FloatingMenu: () => <div data-testid="floating-menu">FloatingMenu</div>,
}));

vi.mock("@/components/mobile/stats/SSEProgressBar", () => ({
  SSEProgressBar: ({ progress, isVisible, onComplete }: any) => (
    <div data-testid="sse-progress-bar">
      Progress: {progress}%, Visible: {String(isVisible)}
    </div>
  ),
}));

vi.mock("@/components/mobile/stats/StatCard", () => ({
  StatCard: ({ label, value, description }: any) => (
    <div data-testid="stat-card">
      <div>{label}</div>
      <div>{value}</div>
      <div>{description}</div>
    </div>
  ),
}));

vi.mock("react-icons/fi", () => ({
  FiRss: () => <div data-testid="rss-icon">RSS</div>,
  FiFileText: () => <div data-testid="file-text-icon">FileText</div>,
  FiLayers: () => <div data-testid="layers-icon">Layers</div>,
}));

describe("FeedsStatsPage - Infinite Reconnection Prevention", () => {
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
    vi.stubEnv("NEXT_PUBLIC_API_BASE_URL", "http://localhost:8080/api");

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
    vi.mocked(setupSSEWithReconnect).mockImplementation(
      mockSetupSSEWithReconnect,
    );
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.unstubAllEnvs();
  });

  describe("SSE Connection Stability", () => {
    it("should create SSE connection only once during component lifecycle", () => {
      const { rerender } = render(<FeedsStatsPage />);

      // Verify initial SSE connection
      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);

      // Trigger re-renders
      rerender(<FeedsStatsPage />);
      rerender(<FeedsStatsPage />);
      rerender(<FeedsStatsPage />);

      // SSE should still be created only once
      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);
    });

    it("should not recreate SSE when progress updates occur", () => {
      render(<FeedsStatsPage />);

      // Get the onData callback
      const onDataCallback = mockSetupSSEWithReconnect.mock.calls[0][1];

      // Initial connection count
      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);

      // Simulate multiple data updates (which trigger progress resets)
      act(() => {
        onDataCallback({ feed_amount: { amount: 1 } });
      });

      act(() => {
        onDataCallback({ feed_amount: { amount: 2 } });
      });

      act(() => {
        onDataCallback({ feed_amount: { amount: 3 } });
      });

      // SSE should still be created only once despite progress resets
      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);
    });

    it("should not recreate SSE when internal timers fire", () => {
      render(<FeedsStatsPage />);

      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);

      // Advance time to trigger internal progress updates
      act(() => {
        vi.advanceTimersByTime(1000);
      });

      act(() => {
        vi.advanceTimersByTime(2000);
      });

      act(() => {
        vi.advanceTimersByTime(5000);
      });

      // SSE should still be created only once
      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);
    });

    it("should properly cleanup SSE on unmount", () => {
      const { unmount } = render(<FeedsStatsPage />);

      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);

      unmount();

      // Cleanup should be called
      expect(mockCleanup).toHaveBeenCalledTimes(1);
    });
  });

  describe("Progress Reset Integration", () => {
    it("should handle progress reset without causing SSE reconnection", () => {
      render(<FeedsStatsPage />);

      const onDataCallback = mockSetupSSEWithReconnect.mock.calls[0][1];

      // Simulate rapid data updates that would trigger progress resets
      for (let i = 0; i < 10; i++) {
        act(() => {
          onDataCallback({
            feed_amount: { amount: i },
            unsummarized_feed: { amount: i * 2 },
            total_articles: { amount: i * 10 },
          });
        });
      }

      // SSE should still be created only once despite multiple progress resets
      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);
    });

    it("should handle concurrent progress updates and timer advances", () => {
      render(<FeedsStatsPage />);

      const onDataCallback = mockSetupSSEWithReconnect.mock.calls[0][1];

      // Interleave data updates and timer advances
      for (let i = 0; i < 5; i++) {
        act(() => {
          onDataCallback({ feed_amount: { amount: i } });
          vi.advanceTimersByTime(500);
        });
      }

      // SSE should still be created only once
      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);
    });
  });

  describe("Real-world Scenarios", () => {
    it("should handle rapid connection state changes without SSE recreation", () => {
      render(<FeedsStatsPage />);

      const onDataCallback = mockSetupSSEWithReconnect.mock.calls[0][1];
      const onErrorCallback = mockSetupSSEWithReconnect.mock.calls[0][2];
      const onOpenCallback = mockSetupSSEWithReconnect.mock.calls[0][4];

      // Simulate rapid connection state changes
      act(() => {
        onOpenCallback(); // Connection opens
        onDataCallback({ feed_amount: { amount: 5 } }); // Data received
        onErrorCallback(); // Connection error
        onOpenCallback(); // Reconnection
        onDataCallback({ feed_amount: { amount: 10 } }); // More data
      });

      // SSE should still be created only once
      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);
    });

    it("should maintain stability under stress conditions", () => {
      render(<FeedsStatsPage />);

      const onDataCallback = mockSetupSSEWithReconnect.mock.calls[0][1];

      // Stress test with many rapid updates
      for (let i = 0; i < 100; i++) {
        act(() => {
          if (i % 10 === 0) {
            vi.advanceTimersByTime(100);
          }
          onDataCallback({
            feed_amount: { amount: Math.random() * 100 },
            unsummarized_feed: { amount: Math.random() * 50 },
            total_articles: { amount: Math.random() * 1000 },
          });
        });
      }

      // SSE should still be created only once even under stress
      expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(1);
    });
  });

  describe("Memory Leak Prevention", () => {
    it("should not accumulate timers or event listeners", () => {
      const { unmount } = render(<FeedsStatsPage />);

      // Get initial timer count
      const initialTimerCount = vi.getTimerCount();
      expect(initialTimerCount).toBeGreaterThan(0);

      // Simulate activity
      act(() => {
        vi.advanceTimersByTime(5000);
      });

      unmount();

      // All timers should be cleaned up
      const finalTimerCount = vi.getTimerCount();
      expect(finalTimerCount).toBe(0);
    });

    it("should handle multiple mount/unmount cycles correctly", () => {
      for (let i = 0; i < 5; i++) {
        const { unmount } = render(<FeedsStatsPage />);

        // Each mount should create one SSE connection
        expect(mockSetupSSEWithReconnect).toHaveBeenCalledTimes(i + 1);

        unmount();

        // Each unmount should call cleanup
        expect(mockCleanup).toHaveBeenCalledTimes(i + 1);
      }
    });
  });

  describe("Rendering Stability", () => {
    it("should render correctly with dynamic data updates", () => {
      const { getAllByText } = render(<FeedsStatsPage />);

      // Should render initial UI
      expect(getAllByText("Feeds Statistics")[0]).toBeTruthy();
      expect(getAllByText("TOTAL FEEDS")[0]).toBeTruthy();
      expect(getAllByText("TOTAL ARTICLES")[0]).toBeTruthy();
      expect(getAllByText("UNSUMMARIZED ARTICLES")[0]).toBeTruthy();

      const onDataCallback = mockSetupSSEWithReconnect.mock.calls[0][1];

      // Update data and verify rendering stability
      act(() => {
        onDataCallback({
          feed_amount: { amount: 25 },
          unsummarized_feed: { amount: 18 },
          total_articles: { amount: 1337 },
        });
      });

      // UI should still be stable
      expect(getAllByText("Feeds Statistics")[0]).toBeTruthy();
      expect(getAllByText("TOTAL FEEDS")[0]).toBeTruthy();
    });
  });
});

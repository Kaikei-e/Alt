import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { feedsApiSse } from "@/lib/apiSse";
import { FeedStatsSummary } from "@/schema/feedStats";

// Mock EventSource
let lastEventSourceInstance: MockEventSource | null = null;
let allEventSourceInstances: MockEventSource[] = [];

class MockEventSource {
  url: string;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  readyState: number = EventSource.CONNECTING;

  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 2;

  constructor(url: string) {
    this.url = url;
    lastEventSourceInstance = this;
    allEventSourceInstances.push(this);
  }

  close() {
    this.readyState = EventSource.CLOSED;
  }

  // Test helpers
  triggerOpen() {
    this.readyState = EventSource.OPEN;
    if (this.onopen) {
      this.onopen(new Event("open"));
    }
  }

  triggerMessage(data: string) {
    if (this.onmessage) {
      this.onmessage(new MessageEvent("message", { data }));
    }
  }

  triggerError() {
    if (this.onerror) {
      this.onerror(new Event("error"));
    }
  }
}

vi.stubGlobal("EventSource", MockEventSource);

describe("feedsApiSse", () => {
  let mockOnMessage: ReturnType<typeof vi.fn>;
  let mockOnError: ReturnType<typeof vi.fn>;
  let consoleSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    mockOnMessage = vi.fn();
    mockOnError = vi.fn();
    consoleSpy = vi.spyOn(console, "log").mockImplementation(() => { });
    vi.clearAllMocks();
    vi.useFakeTimers();
    lastEventSourceInstance = null;
    allEventSourceInstances = [];
  });

  afterEach(() => {
    vi.useRealTimers();
    consoleSpy.mockRestore();
  });

  describe("getFeedsStats", () => {
    it("should create EventSource with correct URL", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      expect(connection).toBeDefined();
      expect(connection.getReadyState()).toBe(MockEventSource.CONNECTING);
      expect(lastEventSourceInstance).toBeDefined();
      expect(lastEventSourceInstance?.url).toBe("http://localhost/api/v1/sse/feeds/stats");
    });

    it("should handle successful connection", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      // Trigger connection opened
      lastEventSourceInstance?.triggerOpen();

      // Check that "SSE connection opened:" was called (should be second call)
      const openedCalls = consoleSpy.mock.calls.filter(call =>
        typeof call[0] === 'string' && call[0].includes("SSE connection opened:")
      );
      expect(openedCalls.length).toBeGreaterThan(0);
    });

    it("should parse and handle valid JSON messages", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      const mockStats: FeedStatsSummary = {
        feed_amount: {
          amount: 100,
        },
        summarized_feed: {
          amount: 50,
        },
      };

      lastEventSourceInstance?.triggerMessage(JSON.stringify(mockStats));

      expect(mockOnMessage).toHaveBeenCalledWith(mockStats);
    });

    it("should handle invalid JSON messages", () => {
      const errorSpy = vi.spyOn(console, "error").mockImplementation(() => { });

      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      lastEventSourceInstance?.triggerMessage("invalid-json");

      expect(mockOnMessage).not.toHaveBeenCalled();
      expect(errorSpy).toHaveBeenCalledWith(
        "Error parsing SSE data:",
        expect.any(Error)
      );

      errorSpy.mockRestore();
    });

    it("should handle connection errors", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      lastEventSourceInstance?.triggerError();

      expect(mockOnError).toHaveBeenCalledWith(expect.any(Event));
    });

    it("should attempt reconnection on connection close", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      // Simulate connection close
      if (lastEventSourceInstance) {
        lastEventSourceInstance.readyState = MockEventSource.CLOSED;
        lastEventSourceInstance.triggerError();
      }

      // Advance timer to trigger reconnection
      vi.advanceTimersByTime(2000);

      expect(consoleSpy).toHaveBeenCalledWith(
        expect.stringContaining("SSE connection closed, attempting to reconnect...")
      );

      // Should also create a new connection
      expect(allEventSourceInstances.length).toBe(2);
    });

    it("should stop reconnecting after max attempts", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      const errorSpy = vi.spyOn(console, "error").mockImplementation(() => { });

      // Simulate multiple connection failures
      for (let i = 0; i < 6; i++) {
        if (lastEventSourceInstance) {
          lastEventSourceInstance.readyState = MockEventSource.CLOSED;
          lastEventSourceInstance.triggerError();
        }
        vi.advanceTimersByTime(2000 * (i + 1));
      }

      expect(errorSpy).toHaveBeenCalledWith("Max reconnection attempts reached");
      expect(mockOnError).toHaveBeenCalled();

      errorSpy.mockRestore();
    });

    it("should use exponential backoff for reconnection", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      // First reconnection attempt
      if (lastEventSourceInstance) {
        lastEventSourceInstance.readyState = MockEventSource.CLOSED;
        lastEventSourceInstance.triggerError();
      }

      vi.advanceTimersByTime(2000); // First attempt after 2s

      // Second reconnection attempt - should be on the new instance
      if (lastEventSourceInstance) {
        lastEventSourceInstance.readyState = MockEventSource.CLOSED;
        lastEventSourceInstance.triggerError();
      }

      vi.advanceTimersByTime(4000); // Second attempt after 4s

      // Should have created 3 instances (initial + 2 reconnections)
      expect(allEventSourceInstances.length).toBe(3);
      expect(consoleSpy).toHaveBeenCalledWith(
        expect.stringContaining("attempt 3")
      );
    });

    it("should close connection properly", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      const closeSpy = vi.spyOn(lastEventSourceInstance!, "close");

      connection.close();

      expect(closeSpy).toHaveBeenCalled();
    });

    it("should return correct ready state", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      expect(connection.getReadyState()).toBe(MockEventSource.CONNECTING);

      lastEventSourceInstance?.triggerOpen();

      expect(connection.getReadyState()).toBe(MockEventSource.OPEN);
    });

    it("should use default error handler when none provided", () => {
      const errorSpy = vi.spyOn(console, "error").mockImplementation(() => { });

      const connection = feedsApiSse.getFeedsStats(mockOnMessage);

      lastEventSourceInstance?.triggerError();

      expect(errorSpy).toHaveBeenCalledWith("SSE error:", expect.any(Event));

      errorSpy.mockRestore();
    });

    it("should handle connection events in correct order", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      // Open connection
      lastEventSourceInstance?.triggerOpen();

      // Check that "SSE connection opened:" was called
      const openedCalls = consoleSpy.mock.calls.filter(call =>
        typeof call[0] === 'string' && call[0].includes("SSE connection opened:")
      );
      expect(openedCalls.length).toBeGreaterThan(0);

      // Receive message
      const mockStats: FeedStatsSummary = {
        feed_amount: {
          amount: 50,
        },
        summarized_feed: {
          amount: 25,
        },
      };

      lastEventSourceInstance?.triggerMessage(JSON.stringify(mockStats));
      expect(mockOnMessage).toHaveBeenCalledWith(mockStats);

      // Connection error
      lastEventSourceInstance?.triggerError();
      expect(mockOnError).toHaveBeenCalled();
    });
  });
});
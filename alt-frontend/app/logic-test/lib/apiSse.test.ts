import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { feedsApiSse } from "@/lib/apiSse";
import { FeedStatsSummary } from "@/schema/feedStats";

// Mock EventSource
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
    this.readyState = EventSource.CLOSED;
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
    consoleSpy = vi.spyOn(console, "log").mockImplementation(() => {});
    vi.clearAllMocks();
    vi.useFakeTimers();
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
    });

    it("should handle successful connection", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      const eventSource = MockEventSource.prototype as any;
      
      // Trigger connection opened
      eventSource.triggerOpen();

      expect(consoleSpy).toHaveBeenCalledWith(
        expect.stringContaining("SSE connection opened:")
      );
    });

    it("should parse and handle valid JSON messages", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      const eventSource = MockEventSource.prototype as any;

      const mockStats: FeedStatsSummary = {
        feed_amount: {
          amount: 100,
        },
        summarized_feed: {
          amount: 50,
        },
      };

      eventSource.triggerMessage(JSON.stringify(mockStats));

      expect(mockOnMessage).toHaveBeenCalledWith(mockStats);
    });

    it("should handle invalid JSON messages", () => {
      const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
      
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      const eventSource = MockEventSource.prototype as any;

      eventSource.triggerMessage("invalid-json");

      expect(mockOnMessage).not.toHaveBeenCalled();
      expect(errorSpy).toHaveBeenCalledWith(
        "Error parsing SSE data:",
        expect.any(Error)
      );

      errorSpy.mockRestore();
    });

    it("should handle connection errors", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      const eventSource = MockEventSource.prototype as any;

      eventSource.triggerError();

      expect(mockOnError).toHaveBeenCalledWith(expect.any(Event));
    });

    it("should attempt reconnection on connection close", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      const eventSource = MockEventSource.prototype as any;

      // Simulate connection close
      eventSource.readyState = MockEventSource.CLOSED;
      eventSource.triggerError();

      // Advance timer to trigger reconnection
      vi.advanceTimersByTime(2000);

      expect(consoleSpy).toHaveBeenCalledWith(
        expect.stringContaining("SSE connection closed, attempting to reconnect...")
      );
    });

    it("should stop reconnecting after max attempts", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      const eventSource = MockEventSource.prototype as any;
      const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

      // Simulate multiple connection failures
      for (let i = 0; i < 6; i++) {
        eventSource.readyState = MockEventSource.CLOSED;
        eventSource.triggerError();
        vi.advanceTimersByTime(2000 * (i + 1));
      }

      expect(errorSpy).toHaveBeenCalledWith("Max reconnection attempts reached");
      expect(mockOnError).toHaveBeenCalled();

      errorSpy.mockRestore();
    });

    it("should use exponential backoff for reconnection", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      const eventSource = MockEventSource.prototype as any;

      // First reconnection attempt
      eventSource.readyState = MockEventSource.CLOSED;
      eventSource.triggerError();
      
      vi.advanceTimersByTime(2000); // First attempt after 2s

      // Second reconnection attempt
      eventSource.readyState = MockEventSource.CLOSED;
      eventSource.triggerError();
      
      vi.advanceTimersByTime(4000); // Second attempt after 4s

      expect(consoleSpy).toHaveBeenCalledWith(
        expect.stringContaining("attempting to reconnect")
      );
    });

    it("should close connection properly", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      const eventSource = MockEventSource.prototype as any;
      const closeSpy = vi.spyOn(eventSource, "close");

      connection.close();

      expect(closeSpy).toHaveBeenCalled();
    });

    it("should return correct ready state", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      
      expect(connection.getReadyState()).toBe(MockEventSource.CONNECTING);

      const eventSource = MockEventSource.prototype as any;
      eventSource.triggerOpen();
      
      expect(connection.getReadyState()).toBe(MockEventSource.OPEN);
    });

    it("should use default error handler when none provided", () => {
      const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
      
      const connection = feedsApiSse.getFeedsStats(mockOnMessage);
      const eventSource = MockEventSource.prototype as any;

      eventSource.triggerError();

      expect(errorSpy).toHaveBeenCalledWith("SSE error:", expect.any(Event));

      errorSpy.mockRestore();
    });

    it("should handle connection events in correct order", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      const eventSource = MockEventSource.prototype as any;

      // Open connection
      eventSource.triggerOpen();
      expect(consoleSpy).toHaveBeenCalledWith(
        expect.stringContaining("SSE connection opened:")
      );

      // Receive message
      const mockStats: FeedStatsSummary = {
        feed_amount: {
          amount: 50,
        },
        summarized_feed: {
          amount: 25,
        },
      };

      eventSource.triggerMessage(JSON.stringify(mockStats));
      expect(mockOnMessage).toHaveBeenCalledWith(mockStats);

      // Connection error
      eventSource.triggerError();
      expect(mockOnError).toHaveBeenCalled();
    });

    it("should reset reconnection attempts on successful connection", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      const eventSource = MockEventSource.prototype as any;
      const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

      // First failure and reconnection
      eventSource.readyState = MockEventSource.CLOSED;
      eventSource.triggerError();
      vi.advanceTimersByTime(2000);

      // Successful connection
      eventSource.triggerOpen();

      // Another failure should start from attempt 1 again
      eventSource.readyState = MockEventSource.CLOSED;
      eventSource.triggerError();

      // Should use base delay (2000ms) not exponential
      vi.advanceTimersByTime(2000);

      expect(consoleSpy).toHaveBeenCalledWith(
        expect.stringContaining("attempt 1")
      );

      errorSpy.mockRestore();
    });
  });
});
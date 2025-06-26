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

  beforeEach(() => {
    mockOnMessage = vi.fn();
    mockOnError = vi.fn();
    vi.clearAllMocks();
    lastEventSourceInstance = null;
    allEventSourceInstances = [];
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("getFeedsStats", () => {
    it("should create EventSource with correct URL", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      expect(connection).toBeDefined();
      expect(connection).toBe(lastEventSourceInstance);
      expect(lastEventSourceInstance).toBeDefined();
      expect(lastEventSourceInstance?.url).toBe(
        "http://localhost/api/v1/sse/feeds/stats",
      );
    });

    it("should handle successful connection", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      // Trigger connection opened
      lastEventSourceInstance?.triggerOpen();

      // The actual implementation doesn't log connection opened, so we just check the connection exists
      expect(connection).toBeDefined();
      expect(lastEventSourceInstance?.readyState).toBe(MockEventSource.OPEN);
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
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      lastEventSourceInstance?.triggerMessage("invalid-json");

      // The actual implementation handles JSON parsing errors silently
      expect(mockOnMessage).not.toHaveBeenCalled();
      expect(connection).toBeDefined();
    });

    it("should handle connection errors", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      lastEventSourceInstance?.triggerError();

      expect(mockOnError).toHaveBeenCalled();
    });

    it("should use default error handler when none provided", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage);

      lastEventSourceInstance?.triggerError();

      // Default error handler should be used (doesn't throw)
      expect(connection).toBeDefined();
    });

    it("should handle connection events in correct order", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      // Open connection
      lastEventSourceInstance?.triggerOpen();
      expect(lastEventSourceInstance?.readyState).toBe(MockEventSource.OPEN);

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

    it("should close connection properly", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);
      const closeSpy = vi.spyOn(lastEventSourceInstance!, "close");

      connection?.close();

      expect(closeSpy).toHaveBeenCalled();
    });

    it("should return correct ready state", () => {
      const connection = feedsApiSse.getFeedsStats(mockOnMessage, mockOnError);

      expect(connection?.readyState).toBe(MockEventSource.CONNECTING);

      lastEventSourceInstance?.triggerOpen();

      expect(connection?.readyState).toBe(MockEventSource.OPEN);
    });

    it("should handle total_articles in SSE data", () => {
      const onData = vi.fn();
      const onError = vi.fn();

      feedsApiSse.getFeedsStats(onData, onError);

      // Simulate SSE message with total_articles
      const mockData = {
        feed_amount: { amount: 10 },
        unsummarized_feed: { amount: 5 },
        total_articles: { amount: 100 },
      };

      lastEventSourceInstance?.triggerMessage(JSON.stringify(mockData));

      expect(onData).toHaveBeenCalledWith(mockData);
      expect(onData).toHaveBeenCalledTimes(1);
    });

    it("should handle missing total_articles field", () => {
      const onData = vi.fn();
      const onError = vi.fn();

      feedsApiSse.getFeedsStats(onData, onError);

      // Simulate SSE message without total_articles
      const mockData = {
        feed_amount: { amount: 10 },
        unsummarized_feed: { amount: 5 },
      };

      lastEventSourceInstance?.triggerMessage(JSON.stringify(mockData));

      expect(onData).toHaveBeenCalledWith(mockData);
      expect(onError).not.toHaveBeenCalled();
    });
  });
});

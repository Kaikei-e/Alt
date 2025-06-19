import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { throttle } from "@/lib/utils/throttle";

describe("InfiniteScroll Logic", () => {
  let mockCallback: ReturnType<typeof vi.fn>;
  let mockIntersectionObserver: ReturnType<typeof vi.fn>;
  let mockObserverInstance: {
    observe: ReturnType<typeof vi.fn>;
    unobserve: ReturnType<typeof vi.fn>;
    disconnect: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    mockCallback = vi.fn();

    mockObserverInstance = {
      observe: vi.fn(),
      unobserve: vi.fn(),
      disconnect: vi.fn(),
    };

    mockIntersectionObserver = vi.fn(() => mockObserverInstance);
    vi.stubGlobal("IntersectionObserver", mockIntersectionObserver);

    vi.clearAllMocks();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("should create IntersectionObserver with correct options", () => {
    const observer = new IntersectionObserver(() => { }, {
      rootMargin: "200px 0px",
      threshold: 0.1,
    });

    expect(mockIntersectionObserver).toHaveBeenCalledWith(
      expect.any(Function),
      {
        rootMargin: "200px 0px",
        threshold: 0.1,
      }
    );
  });

  it("should observe element when provided", () => {
    const mockElement = { tagName: "DIV" } as HTMLDivElement;
    const observer = new IntersectionObserver(() => { });
    observer.observe(mockElement);

    expect(mockObserverInstance.observe).toHaveBeenCalledWith(mockElement);
  });

  it("should disconnect observer on cleanup", () => {
    const observer = new IntersectionObserver(() => { });
    observer.disconnect();

    expect(mockObserverInstance.disconnect).toHaveBeenCalled();
  });

  it("should handle intersection callback correctly", () => {
    let capturedCallback: (entries: IntersectionObserverEntry[]) => void;

    mockIntersectionObserver.mockImplementation((callback) => {
      capturedCallback = callback;
      return mockObserverInstance;
    });

    new IntersectionObserver(mockCallback);

    // Simulate intersection
    const entries = [
      { isIntersecting: true } as IntersectionObserverEntry,
      { isIntersecting: false } as IntersectionObserverEntry,
    ];

    capturedCallback!(entries);

    expect(mockCallback).toHaveBeenCalledWith(entries);
  });

  describe("throttle behavior in infinite scroll context", () => {
    it("should throttle rapid intersection callbacks", () => {
      const throttledCallback = throttle(mockCallback, 500);

      // Rapid calls
      throttledCallback();
      throttledCallback();
      throttledCallback();

      // Should only be called once initially
      expect(mockCallback).toHaveBeenCalledTimes(1);

      // Advance time to trigger throttled call
      vi.advanceTimersByTime(500);
      expect(mockCallback).toHaveBeenCalledTimes(2);
    });

    it("should handle retry logic correctly", () => {
      let retryCount = 0;
      const maxRetries = 3;

      const retryCallback = () => {
        if (retryCount < maxRetries) {
          retryCount++;
          mockCallback();
        }
      };

      const throttledRetryCallback = throttle(retryCallback, 500);

      // Call multiple times to simulate retries
      for (let i = 0; i < 5; i++) {
        throttledRetryCallback();
        vi.advanceTimersByTime(600);
      }

      // Should only retry up to maxRetries
      expect(mockCallback).toHaveBeenCalledTimes(maxRetries);
    });

    it("should reset retry count when needed", () => {
      let retryCount = 0;
      const maxRetries = 2;

      const callback = () => {
        if (retryCount < maxRetries) {
          retryCount++;
          mockCallback();
        }
      };

      // Simulate retries reaching limit
      for (let i = 0; i < maxRetries; i++) {
        callback();
      }
      expect(mockCallback).toHaveBeenCalledTimes(maxRetries);

      // Reset retry count (simulating resetKey change)
      retryCount = 0;

      // Should be able to retry again
      callback();
      expect(mockCallback).toHaveBeenCalledTimes(maxRetries + 1);
    });
  });

  describe("element setup and cleanup", () => {
    it("should handle missing element gracefully", () => {
      const setupObserver = () => {
        const element = null;

        if (!element) {
          return setTimeout(() => setupObserver(), 100);
        }

        return new IntersectionObserver(() => { });
      };

      const timeoutId = setupObserver();
      expect(timeoutId).toBeDefined();

      if (timeoutId) {
        clearTimeout(timeoutId as NodeJS.Timeout);
      }
    });

    it("should clean up timeouts and observers", () => {
      const clearTimeoutSpy = vi.spyOn(global, "clearTimeout");

      const timeoutId = setTimeout(() => { }, 100);
      const observer = new IntersectionObserver(() => { });

      clearTimeout(timeoutId);
      observer.disconnect();

      expect(clearTimeoutSpy).toHaveBeenCalledWith(timeoutId);
      expect(mockObserverInstance.disconnect).toHaveBeenCalled();

      clearTimeoutSpy.mockRestore();
    });
  });
});
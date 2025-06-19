import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { throttle, debounce, createRateLimiter } from "@/lib/utils/throttle";

describe("throttle utility", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("throttle", () => {
    it("should call function immediately on first call", () => {
      const fn = vi.fn();
      const throttledFn = throttle(fn, 1000);

      throttledFn();

      expect(fn).toHaveBeenCalledTimes(1);
    });

    it("should throttle subsequent calls within delay period", () => {
      const fn = vi.fn();
      const throttledFn = throttle(fn, 1000);

      throttledFn();
      throttledFn();
      throttledFn();

      expect(fn).toHaveBeenCalledTimes(1);
    });

    it("should call function again after delay period", () => {
      const fn = vi.fn();
      const throttledFn = throttle(fn, 1000);

      throttledFn();
      expect(fn).toHaveBeenCalledTimes(1);

      vi.advanceTimersByTime(1100);
      throttledFn();
      expect(fn).toHaveBeenCalledTimes(2);
    });

    it("should execute pending call after delay", () => {
      const fn = vi.fn();
      const throttledFn = throttle(fn, 1000);

      throttledFn();
      throttledFn(); // This should be scheduled

      expect(fn).toHaveBeenCalledTimes(1);

      vi.advanceTimersByTime(1000);
      expect(fn).toHaveBeenCalledTimes(2);
    });

    it("should pass arguments correctly", () => {
      const fn = vi.fn();
      const throttledFn = throttle(fn, 1000);

      throttledFn("arg1", "arg2");

      expect(fn).toHaveBeenCalledWith("arg1", "arg2");
    });

    it("should handle multiple rapid calls correctly", () => {
      const fn = vi.fn();
      const throttledFn = throttle(fn, 1000);

      // Rapid calls
      throttledFn("call1");
      throttledFn("call2");
      throttledFn("call3");
      throttledFn("call4");

      expect(fn).toHaveBeenCalledTimes(1);
      expect(fn).toHaveBeenCalledWith("call1");

      // Advance time to trigger scheduled call
      vi.advanceTimersByTime(1000);
      expect(fn).toHaveBeenCalledTimes(2);
      expect(fn).toHaveBeenLastCalledWith("call4");
    });

    it("should cancel previous scheduled call when new call comes in", () => {
      const fn = vi.fn();
      const throttledFn = throttle(fn, 1000);

      throttledFn("first");
      throttledFn("second");
      
      vi.advanceTimersByTime(500);
      throttledFn("third");

      // At this point: 
      // - "first" was called immediately
      // - "second" was scheduled but cancelled by "third"
      // - "third" is now scheduled for 500ms from when it was called
      expect(fn).toHaveBeenCalledTimes(1);
      expect(fn).toHaveBeenCalledWith("first");

      // Advance to when the latest scheduled call should execute
      vi.advanceTimersByTime(500);
      expect(fn).toHaveBeenCalledTimes(2);
      expect(fn).toHaveBeenLastCalledWith("third");
    });
  });

  describe("debounce", () => {
    it("should not call function immediately", () => {
      const fn = vi.fn();
      const debouncedFn = debounce(fn, 1000);

      debouncedFn();

      expect(fn).not.toHaveBeenCalled();
    });

    it("should call function after delay", () => {
      const fn = vi.fn();
      const debouncedFn = debounce(fn, 1000);

      debouncedFn();
      vi.advanceTimersByTime(1000);

      expect(fn).toHaveBeenCalledTimes(1);
    });

    it("should reset delay on subsequent calls", () => {
      const fn = vi.fn();
      const debouncedFn = debounce(fn, 1000);

      debouncedFn();
      vi.advanceTimersByTime(500);
      debouncedFn(); // This should reset the timer

      vi.advanceTimersByTime(500);
      expect(fn).not.toHaveBeenCalled();

      vi.advanceTimersByTime(500);
      expect(fn).toHaveBeenCalledTimes(1);
    });

    it("should only call function once for multiple rapid calls", () => {
      const fn = vi.fn();
      const debouncedFn = debounce(fn, 1000);

      debouncedFn("call1");
      debouncedFn("call2");
      debouncedFn("call3");

      vi.advanceTimersByTime(1000);

      expect(fn).toHaveBeenCalledTimes(1);
      expect(fn).toHaveBeenCalledWith("call3"); // Should use last arguments
    });

    it("should pass arguments correctly", () => {
      const fn = vi.fn();
      const debouncedFn = debounce(fn, 1000);

      debouncedFn("arg1", "arg2");
      vi.advanceTimersByTime(1000);

      expect(fn).toHaveBeenCalledWith("arg1", "arg2");
    });
  });

  describe("createRateLimiter", () => {
    beforeEach(() => {
      vi.useRealTimers(); // Rate limiter uses real time
    });

    it("should allow calls up to the limit", () => {
      const rateLimiter = createRateLimiter(3, 1000);

      expect(rateLimiter()).toBe(true);
      expect(rateLimiter()).toBe(true);
      expect(rateLimiter()).toBe(true);
      expect(rateLimiter()).toBe(false); // Fourth call should be blocked
    });

    it("should reset after time window", async () => {
      const rateLimiter = createRateLimiter(2, 100);

      expect(rateLimiter()).toBe(true);
      expect(rateLimiter()).toBe(true);
      expect(rateLimiter()).toBe(false);

      // Wait for window to pass
      await new Promise(resolve => setTimeout(resolve, 150));

      expect(rateLimiter()).toBe(true);
      expect(rateLimiter()).toBe(true);
      expect(rateLimiter()).toBe(false);
    });

    it("should handle sliding window correctly", async () => {
      const rateLimiter = createRateLimiter(2, 200);

      expect(rateLimiter()).toBe(true); // t=0
      
      await new Promise(resolve => setTimeout(resolve, 100));
      expect(rateLimiter()).toBe(true); // t=100
      expect(rateLimiter()).toBe(false); // t=100, limit reached

      await new Promise(resolve => setTimeout(resolve, 110)); // t=210
      // First call (t=0) should be outside window now
      expect(rateLimiter()).toBe(true);
    });

    it("should work with different limits and windows", () => {
      const strictLimiter = createRateLimiter(1, 1000);
      const lenientLimiter = createRateLimiter(10, 1000);

      expect(strictLimiter()).toBe(true);
      expect(strictLimiter()).toBe(false);

      for (let i = 0; i < 10; i++) {
        expect(lenientLimiter()).toBe(true);
      }
      expect(lenientLimiter()).toBe(false);
    });

    it("should handle zero limit", () => {
      const noCallsLimiter = createRateLimiter(0, 1000);
      expect(noCallsLimiter()).toBe(false);
    });

    it("should handle very short time windows", async () => {
      const shortWindowLimiter = createRateLimiter(2, 10);

      expect(shortWindowLimiter()).toBe(true);
      expect(shortWindowLimiter()).toBe(true);
      expect(shortWindowLimiter()).toBe(false);

      await new Promise(resolve => setTimeout(resolve, 15));
      expect(shortWindowLimiter()).toBe(true);
    });
  });
});
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import React from "react";
import { useSSEProgress } from "@/hooks/useSSEProgress";

describe("useSSEProgress Hook - Stability Tests", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("Function Stability - NO Infinite Loops", () => {
    it("should have stable reset function across re-renders", () => {
      const { result, rerender } = renderHook(() => useSSEProgress(5000));

      const firstResetFn = result.current.reset;

      // Trigger multiple re-renders
      rerender();
      rerender();
      rerender();

      const secondResetFn = result.current.reset;

      // reset function should be stable (same reference)
      expect(firstResetFn).toBe(secondResetFn);
    });

    it("should not trigger infinite useEffect calls", () => {
      const mockCallback = vi.fn();

      // Create a component that would trigger useEffect if reset function changes
      const TestComponent = ({
        onResetChange,
      }: {
        onResetChange: (fn: any) => void;
      }) => {
        const { reset } = useSSEProgress(5000);
        onResetChange(reset);
        return null;
      };

      const { rerender } = renderHook(
        ({ onResetChange }) => TestComponent({ onResetChange }),
        {
          initialProps: { onResetChange: mockCallback },
        },
      );

      // Should only be called once on initial render
      expect(mockCallback).toHaveBeenCalledTimes(1);

      // Trigger re-renders
      rerender({ onResetChange: mockCallback });
      rerender({ onResetChange: mockCallback });
      rerender({ onResetChange: mockCallback });

      // Should still only be called once if reset function is stable
      expect(mockCallback).toHaveBeenCalledTimes(4); // 1 initial + 3 rerenders

      // But all calls should have the same function reference
      const firstCall = mockCallback.mock.calls[0][0];
      const lastCall = mockCallback.mock.calls[3][0];
      expect(firstCall).toBe(lastCall);
    });

    it("should not recreate intervals unnecessarily", () => {
      const { result, rerender } = renderHook(() => useSSEProgress(5000));

      // Get initial timer count
      const initialTimerCount = vi.getTimerCount();

      // Trigger multiple re-renders
      rerender();
      rerender();
      rerender();

      // Timer count should remain stable (not create new timers for each re-render)
      const finalTimerCount = vi.getTimerCount();
      expect(finalTimerCount).toBe(initialTimerCount);
    });
  });

  describe("Reset Function Behavior", () => {
    it("should reset progress correctly without dependencies", () => {
      const { result } = renderHook(() => useSSEProgress(5000));

      // Start progress
      act(() => {
        vi.advanceTimersByTime(1000);
      });

      expect(result.current.progress).toBeGreaterThan(0);

      // Reset progress
      act(() => {
        result.current.reset();
      });

      expect(result.current.progress).toBe(0);

      // Progress should continue after reset
      act(() => {
        vi.advanceTimersByTime(1000);
      });

      expect(result.current.progress).toBeGreaterThan(0);
    });

    it("should handle multiple rapid resets without issues", () => {
      const { result } = renderHook(() => useSSEProgress(5000));

      // Perform rapid resets
      for (let i = 0; i < 10; i++) {
        act(() => {
          result.current.reset();
        });
      }

      expect(result.current.progress).toBe(0);

      // Should still work after rapid resets
      act(() => {
        vi.advanceTimersByTime(1000);
      });

      expect(result.current.progress).toBeGreaterThan(0);
    });
  });

  describe("Memory Leak Prevention", () => {
    it("should cleanup timers on unmount", () => {
      const { unmount } = renderHook(() => useSSEProgress(5000));

      const timerCountBeforeUnmount = vi.getTimerCount();
      expect(timerCountBeforeUnmount).toBeGreaterThan(0);

      unmount();

      // Timers should be cleaned up
      const timerCountAfterUnmount = vi.getTimerCount();
      expect(timerCountAfterUnmount).toBe(0);
    });

    it("should not update state after unmount", () => {
      const { result, unmount } = renderHook(() => useSSEProgress(5000));

      const progressBeforeUnmount = result.current.progress;

      unmount();

      // Try to advance timers after unmount
      act(() => {
        vi.advanceTimersByTime(1000);
      });

      // Progress should not have changed (component is unmounted)
      expect(result.current.progress).toBe(progressBeforeUnmount);
    });
  });

  describe("Edge Cases", () => {
    it("should handle zero interval gracefully", () => {
      const { result } = renderHook(() => useSSEProgress(0));

      act(() => {
        vi.advanceTimersByTime(100);
      });

      // Should handle zero interval without crashing
      expect(result.current.progress).toBeGreaterThanOrEqual(0);
    });

    it("should handle interval changes without creating infinite loops", () => {
      const { result, rerender } = renderHook(
        ({ interval }) => useSSEProgress(interval),
        { initialProps: { interval: 5000 } },
      );

      const initialTimerCount = vi.getTimerCount();

      // Change interval
      rerender({ interval: 3000 });

      // Should recreate timer once for the interval change
      const finalTimerCount = vi.getTimerCount();
      expect(finalTimerCount).toBeGreaterThanOrEqual(initialTimerCount);

      // But reset function should still be stable
      const resetFn = result.current.reset;
      rerender({ interval: 3000 }); // Same interval, no change

      expect(result.current.reset).toBe(resetFn);
    });
  });

  describe("Performance", () => {
    it("should not cause excessive re-renders in parent components", () => {
      let renderCount = 0;

      const TestParent = () => {
        renderCount++;
        const { reset } = useSSEProgress(5000);

        // Simulate using reset in useEffect (common pattern)
        React.useEffect(() => {
          // This would cause infinite loops if reset is not stable
        }, [reset]);

        return null;
      };

      renderHook(() => TestParent());

      // Advance time to trigger internal timer updates
      act(() => {
        vi.advanceTimersByTime(1000);
      });

      // Should not cause excessive re-renders due to unstable reset function
      expect(renderCount).toBeLessThan(5); // Allow for some initial renders
    });
  });
});

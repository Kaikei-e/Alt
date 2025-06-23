/**
 * @vitest-environment jsdom
 */
import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useSSEProgress } from "@/hooks/useSSEProgress";

describe("useSSEProgress Hook - REAL BROWSER BEHAVIOR", () => {
  beforeEach(() => {
    // Use REAL timers to catch actual timing issues
    vi.useRealTimers();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("ACTUAL PROBLEMATIC SCENARIOS", () => {
    it("should reproduce the mid-cycle reset bug with REAL timers", async () => {
      const { result } = renderHook(() => useSSEProgress(1000)); // 1 second for faster test

      // Wait for real progress (no fake timers)
      await new Promise(resolve => setTimeout(resolve, 400)); // 40% progress

      const progressAt400ms = result.current.progress;
      expect(progressAt400ms).toBeGreaterThan(35);
      expect(progressAt400ms).toBeLessThan(45);

      // Continue without any reset call - this should NOT reset randomly
      await new Promise(resolve => setTimeout(resolve, 200)); // 60% total

      const progressAt600ms = result.current.progress;
      console.log(`Progress at 400ms: ${progressAt400ms}%, at 600ms: ${progressAt600ms}%`);

      // This is where the bug shows - progress might reset unexpectedly
      if (progressAt600ms < progressAt400ms) {
        console.log("🐛 BUG REPRODUCED: Progress went backwards!");
        console.log(`Expected: ${progressAt600ms} >= ${progressAt400ms}`);
      }

      expect(progressAt600ms).toBeGreaterThanOrEqual(progressAt400ms);
    }, 10000);

    it("should expose useEffect dependency issues with parent component", async () => {
      // Simulate the stats page component behavior
      let resetFunctionChanges = 0;
      let sseReconnections = 0;

      const { result, rerender } = renderHook(() => useSSEProgress(1000));

      let lastResetFunction = result.current.reset;

      // Simulate component re-renders that happen in real usage
      for (let i = 0; i < 5; i++) {
        await new Promise(resolve => setTimeout(resolve, 50));
        rerender();

        if (result.current.reset !== lastResetFunction) {
          resetFunctionChanges++;
          sseReconnections++; // Each reset function change causes SSE reconnection
          console.log(`🐛 Reset function changed on render ${i}!`);
        }
        lastResetFunction = result.current.reset;
      }

      console.log(`Reset function changes: ${resetFunctionChanges}`);
      console.log(`SSE reconnections: ${sseReconnections}`);

      // This should be 0 - stable function reference
      expect(resetFunctionChanges).toBe(0);
    }, 5000);

    it("should show timer overlap issues", async () => {
      const { result } = renderHook(() => useSSEProgress(1000));

      // Let it run for 300ms
      await new Promise(resolve => setTimeout(resolve, 300));
      const progress300 = result.current.progress;

      // Reset manually
      act(() => result.current.reset());
      expect(result.current.progress).toBe(0);

      // Wait another 300ms - should be ~30%, not 60%
      await new Promise(resolve => setTimeout(resolve, 300));
      const progressAfterReset = result.current.progress;

      console.log(`Progress before reset: ${progress300}%`);
      console.log(`Progress after reset + 300ms: ${progressAfterReset}%`);

      // If there are overlapping timers, this might be wrong
      const expectedProgress = 30; // 300ms of 1000ms = 30%
      const tolerance = 10; // Increase tolerance for real-world timing variations

      if (Math.abs(progressAfterReset - expectedProgress) > tolerance) {
        console.log(`🐛 Possible timer overlap! Expected ~${expectedProgress}%, got ${progressAfterReset}%`);
      }

      expect(progressAfterReset).toBeCloseTo(expectedProgress, -1); // Use -1 for more tolerance
    }, 5000);

    it("should catch the actual useCallback dependency bug", () => {
      // Test if the reset function has ANY dependencies that change
      let intervalMs = 1000;

      const { result, rerender } = renderHook(() => useSSEProgress(intervalMs));
      const originalReset = result.current.reset;

      // Change the interval - this might recreate the reset function
      intervalMs = 2000;
      rerender();

      const newReset = result.current.reset;

      if (originalReset !== newReset) {
        console.log("🐛 FOUND THE BUG! Reset function recreated when intervalMs changed!");
        console.log("This causes SSE connection to restart in parent component!");

        // Log the function source to see what's different
        console.log("Original function:", originalReset.toString());
        console.log("New function:", newReset.toString());
      }

      // This should pass if implemented correctly
      expect(newReset).toBe(originalReset);
    });
  });

  describe("SIMULATE ACTUAL STATS PAGE USAGE", () => {
    it("should work exactly like the stats page component", async () => {
      // Simulate the exact usage pattern from stats page
      const mockSSEMessages = [
        { timestamp: 500, data: { feed_amount: { amount: 10 } } },
        { timestamp: 1200, data: { feed_amount: { amount: 15 } } }, // Early message
        { timestamp: 2800, data: { feed_amount: { amount: 20 } } }, // Another early message
      ];

      const { result } = renderHook(() => useSSEProgress(5000)); // 5 second cycle like real app

      let messageIndex = 0;
      const processNextMessage = async () => {
        if (messageIndex < mockSSEMessages.length) {
          const message = mockSSEMessages[messageIndex++];

          await new Promise(resolve => setTimeout(resolve, message.timestamp));

          console.log(`SSE message at ${message.timestamp}ms: resetting progress`);
          act(() => result.current.reset()); // This is what happens when SSE data arrives
        }
      };

      // Start processing messages
      const messageProcessing = processNextMessage();

      // Monitor progress continuously
      const progressLog: Array<{time: number, progress: number}> = [];

      for (let i = 0; i < 30; i++) {
        await new Promise(resolve => setTimeout(resolve, 100));
        progressLog.push({
          time: i * 100,
          progress: result.current.progress
        });
      }

      await messageProcessing;

      // Analyze the progress log for odd behavior
      console.log("Progress timeline:");
      progressLog.forEach(entry => {
        console.log(`${entry.time}ms: ${entry.progress.toFixed(1)}%`);
      });

      // Check for any backwards progress (the "snapping" issue)
      for (let i = 1; i < progressLog.length; i++) {
        const prev = progressLog[i - 1];
        const curr = progressLog[i];

        // Allow for resets due to SSE messages, but not random decreases
        // Increase tolerance for concurrent execution timing variations
        const isExpectedReset = mockSSEMessages.some(msg =>
          Math.abs(msg.timestamp - curr.time) < 150 && curr.progress === 0
        );

        if (curr.progress < prev.progress && !isExpectedReset) {
          console.log(`🐛 Unexpected progress decrease at ${curr.time}ms: ${prev.progress}% → ${curr.progress}%`);
          expect(curr.progress).toBeGreaterThanOrEqual(prev.progress);
        }
      }
    }, 10000);
  });
});
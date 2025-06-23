import { useState, useEffect, useCallback, useRef } from "react";

export const useSSEProgress = (intervalMs: number = 5000) => {
  const [progress, setProgress] = useState(0);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const startTimeRef = useRef<number>(Date.now());
  const intervalMsRef = useRef<number>(intervalMs);
  const isMountedRef = useRef<boolean>(true);

  // Update interval ref when prop changes
  useEffect(() => {
    intervalMsRef.current = intervalMs;
  }, [intervalMs]);

  // Clear any existing timer
  const clearTimer = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  // Create timer update function
  const createTimerUpdate = useCallback(() => {
    return () => {
      // Don't update state if component is unmounted
      if (!isMountedRef.current) {
        if (intervalRef.current) {
          clearInterval(intervalRef.current);
          intervalRef.current = null;
        }
        return;
      }

      const elapsed = Date.now() - startTimeRef.current;
      const newProgress = Math.min((elapsed / intervalMsRef.current) * 100, 100);
      setProgress(newProgress);
    };
  }, []);

  // Start a new timer - internal function with proper cleanup check
  const startNewTimer = useCallback(() => {
    startTimeRef.current = Date.now();
    const updateFn = createTimerUpdate();
    intervalRef.current = setInterval(updateFn, 50);
  }, [createTimerUpdate]);

  // Start the progress timer
  const startTimer = useCallback(() => {
    clearTimer(); // Always clear first
    startNewTimer();
  }, [clearTimer, startNewTimer]);

  // Stable reset function - NO dependencies that change, uses refs only
  const reset = useCallback(() => {
    // Clear any existing timer immediately to prevent race conditions
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }

    // Don't update state if component is unmounted
    if (!isMountedRef.current) {
      return;
    }

    // Reset progress and start time immediately and synchronously
    setProgress(0);
    startTimeRef.current = Date.now();

    // Start new timer immediately to ensure no timing gaps
    intervalRef.current = setInterval(() => {
      // Don't update state if component is unmounted
      if (!isMountedRef.current) {
        if (intervalRef.current) {
          clearInterval(intervalRef.current);
          intervalRef.current = null;
        }
        return;
      }

      const elapsed = Date.now() - startTimeRef.current;
      const newProgress = Math.min((elapsed / intervalMsRef.current) * 100, 100);
      setProgress(newProgress);
    }, 50);
  }, []); // NO dependencies - completely stable

  // Initial setup and cleanup
  useEffect(() => {
    isMountedRef.current = true;
    startTimer();

    return () => {
      isMountedRef.current = false;
      clearTimer();
    };
  }, [intervalMs, startTimer, clearTimer]); // Restart when interval changes

  return { progress, reset };
};
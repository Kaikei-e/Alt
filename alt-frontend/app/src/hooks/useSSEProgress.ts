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

  // Stable clear function - no dependencies
  const clearTimer = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  // Stable update function - no dependencies, uses refs only
  const updateProgress = useCallback(() => {
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
  }, []);

  // Stable start function - no dependencies that change
  const startTimer = useCallback(() => {
    // Clear any existing timer immediately
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }

    // Don't start if component is unmounted
    if (!isMountedRef.current) {
      return;
    }

    // Reset start time and progress
    startTimeRef.current = Date.now();
    setProgress(0);

    // Start new timer
    intervalRef.current = setInterval(updateProgress, 50);
  }, [updateProgress]);

  // Stable reset function - NO dependencies to prevent infinite re-renders
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

    // Start new timer immediately using refs to avoid dependencies
    intervalRef.current = setInterval(() => {
      if (!isMountedRef.current) {
        if (intervalRef.current) {
          clearInterval(intervalRef.current);
          intervalRef.current = null;
        }
        return;
      }

      const elapsed = Date.now() - startTimeRef.current;
      const newProgress = Math.min(
        (elapsed / intervalMsRef.current) * 100,
        100,
      );
      setProgress(newProgress);
    }, 50);
  }, []); // 🔧 FIX: No dependencies to prevent infinite loops

  // Initial setup and cleanup - only restart when intervalMs changes
  useEffect(() => {
    isMountedRef.current = true;
    startTimer();

    return () => {
      isMountedRef.current = false;
      clearTimer();
    };
  }, [intervalMs]); // 🔧 FIX: Only depend on intervalMs to prevent excessive re-runs

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      isMountedRef.current = false;
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  }, []);

  return { progress, reset };
};

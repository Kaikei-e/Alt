import { useState, useEffect, useCallback } from "react";

export const useSSEProgress = (intervalMs: number = 5000) => {
  const [progress, setProgress] = useState(0);

  const reset = useCallback(() => {
    setProgress(0);
  }, []);

  useEffect(() => {
    const startTime = Date.now();

    const interval = setInterval(() => {
      const elapsed = Date.now() - startTime;
      const newProgress = Math.min((elapsed / intervalMs) * 100, 100);
      setProgress(newProgress);
    }, 50); // Update every 50ms for smooth animation

    return () => clearInterval(interval);
  }, [intervalMs]);

  return { progress, reset };
};
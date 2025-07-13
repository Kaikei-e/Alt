import { useEffect, useRef } from "react";

export interface VirtualizationMetrics {
  renderTime: number;
  itemCount: number;
  memoryUsage: number;
  scrollPerformance: number;
}

export const useVirtualizationMetrics = (
  enabled: boolean = true,
  itemCount: number = 0,
) => {
  const metricsRef = useRef<VirtualizationMetrics>({
    renderTime: 0,
    itemCount: 0,
    memoryUsage: 0,
    scrollPerformance: 0,
  });

  useEffect(() => {
    if (!enabled) return;

    const startTime = performance.now();

    // Update metrics
    metricsRef.current = {
      renderTime: performance.now() - startTime,
      itemCount,
      memoryUsage:
        (performance as unknown as { memory?: { usedJSHeapSize?: number } })
          .memory?.usedJSHeapSize || 0,
      scrollPerformance: 0,
    };

    // Log metrics in development
    if (process.env.NODE_ENV === "development") {
      console.log("Virtualization Metrics:", metricsRef.current);
    }
  }, [enabled, itemCount]);

  return metricsRef.current;
};

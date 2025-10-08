import { useEffect, useState } from "react";
import {
  PerformanceThresholdAnalyzer,
  PerformanceMetrics,
} from "@/utils/performanceUtils";

export interface PerformanceData {
  renderTime: number;
  itemCount: number;
  shouldVirtualize: boolean;
}

export const usePerformanceMonitor = (itemCount: number) => {
  const [performanceData, setPerformanceData] =
    useState<PerformanceData | null>(null);
  const [renderStartTime] = useState(() => performance.now());

  useEffect(() => {
    const renderTime = performance.now() - renderStartTime;

    // Get memory information if available (Chrome only)
    const memoryInfo = (
      performance as unknown as { memory?: { usedJSHeapSize?: number } }
    ).memory;
    const memoryUsage = memoryInfo?.usedJSHeapSize || 0;

    // Count DOM nodes
    const domNodeCount = document.querySelectorAll("*").length;

    const metrics: PerformanceMetrics = {
      renderTime,
      scrollTime: 0, // Will be measured separately
      memoryUsage,
      domNodeCount,
    };

    const shouldVirtualize =
      PerformanceThresholdAnalyzer.shouldUseVirtualization(itemCount, metrics);

    const performanceDataResult: PerformanceData = {
      renderTime,
      itemCount,
      shouldVirtualize,
    };

    setPerformanceData(performanceDataResult);

    // Log performance data in development mode
    if (process.env.NODE_ENV === "development") {
    }

    // Performance monitoring without external analytics
    if (process.env.NODE_ENV === "production") {
      // Log performance data for internal monitoring only
    }
  }, [itemCount, renderStartTime]);

  return performanceData;
};

// Hook for measuring scroll performance
export const useScrollPerformanceMonitor = (
  containerRef: React.RefObject<HTMLElement>,
) => {
  const [scrollPerformance, setScrollPerformance] = useState<{
    averageScrollTime: number;
    maxScrollTime: number;
    scrollEvents: number;
  }>({
    averageScrollTime: 0,
    maxScrollTime: 0,
    scrollEvents: 0,
  });

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    let scrollTimes: number[] = [];
    let lastScrollTime = 0;

    const handleScroll = () => {
      const currentTime = performance.now();

      if (lastScrollTime > 0) {
        const scrollTime = currentTime - lastScrollTime;
        scrollTimes.push(scrollTime);

        // Keep only recent scroll times (last 10 events)
        if (scrollTimes.length > 10) {
          scrollTimes = scrollTimes.slice(-10);
        }

        const averageScrollTime =
          scrollTimes.reduce((sum, time) => sum + time, 0) / scrollTimes.length;
        const maxScrollTime = Math.max(...scrollTimes);

        setScrollPerformance({
          averageScrollTime,
          maxScrollTime,
          scrollEvents: scrollTimes.length,
        });
      }

      lastScrollTime = currentTime;
    };

    container.addEventListener("scroll", handleScroll, { passive: true });

    return () => {
      container.removeEventListener("scroll", handleScroll);
    };
  }, [containerRef]);

  return scrollPerformance;
};

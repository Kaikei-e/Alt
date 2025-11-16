import { useEffect, useRef } from "react";

interface DynamicSizingMetrics {
  measurementCount: number;
  averageMeasurementTime: number;
  layoutShiftCount: number;
  errorCount: number;
}

export const useDynamicSizingMetrics = (
  isEnabled: boolean,
  itemCount: number,
) => {
  const metricsRef = useRef<DynamicSizingMetrics>({
    measurementCount: 0,
    averageMeasurementTime: 0,
    layoutShiftCount: 0,
    errorCount: 0,
  });

  useEffect(() => {
    if (!isEnabled) return;

    // Layout Shift Observer
    const observer = new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        if (entry.entryType === "layout-shift") {
          metricsRef.current.layoutShiftCount++;
        }
      }
    });

    observer.observe({ entryTypes: ["layout-shift"] });

    return () => observer.disconnect();
  }, [isEnabled]);

  useEffect(() => {
    if (!isEnabled) return;

    // 定期的にメトリクスを報告
    const interval = setInterval(() => {
      const metrics = metricsRef.current;

      if (process.env.NODE_ENV === "development") {
      }

      // 本番環境での監視（内部ログのみ）
      if (process.env.NODE_ENV === "production") {
      }
    }, 30000); // 30秒ごと

    return () => clearInterval(interval);
  }, [isEnabled, itemCount]);

  return metricsRef.current;
};

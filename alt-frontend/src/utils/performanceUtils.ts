/**
 * Performance utilities for optimizing expensive operations
 */

// Debounce function to limit expensive operations
export function debounce<T extends (...args: unknown[]) => unknown>(
  func: T,
  wait: number,
): (...args: Parameters<T>) => void {
  let timeout: NodeJS.Timeout | null = null;

  return function executedFunction(...args: Parameters<T>) {
    const later = () => {
      if (timeout) {
        clearTimeout(timeout);
        timeout = null;
      }
      func(...args);
    };

    if (timeout) {
      clearTimeout(timeout);
    }
    timeout = setTimeout(later, wait);
  };
}

// Throttle function to limit frequency of expensive operations
export function throttle<T extends (...args: unknown[]) => unknown>(
  func: T,
  limit: number,
): (...args: Parameters<T>) => void {
  let inThrottle: boolean = false;

  return function executedFunction(...args: Parameters<T>) {
    if (!inThrottle) {
      func(...args);
      inThrottle = true;
      setTimeout(() => (inThrottle = false), limit);
    }
  };
}

// Measure performance of operations
export function measurePerformance<T>(operation: () => T, label?: string): T {
  const _startTime = performance.now();
  const result = operation();
  const _endTime = performance.now();

  if (label) {
  }

  return result;
}

// Batch operations for better performance
export function batchOperations<T, R>(
  items: T[],
  batchSize: number,
  operation: (batch: T[]) => R[],
): R[] {
  const results: R[] = [];

  for (let i = 0; i < items.length; i += batchSize) {
    const batch = items.slice(i, i + batchSize);
    const batchResults = operation(batch);
    results.push(...batchResults);
  }

  return results;
}

// Simple memoization for expensive computations
export function memoize<T extends (...args: unknown[]) => unknown>(
  fn: T,
  getKey?: (...args: Parameters<T>) => string,
): T {
  const cache = new Map<string, unknown>();

  return ((...args: Parameters<T>): unknown => {
    const key = getKey ? getKey(...args) : JSON.stringify(args);

    if (cache.has(key)) {
      return cache.get(key)!;
    }

    const result = fn(...args);
    cache.set(key, result);
    return result;
  }) as T;
}

// Clear memoization cache when needed
export function clearMemoCache() {
  // This would clear any global caches if implemented
}

// TASK1: Performance Threshold Analysis for Virtualization
export interface PerformanceMetrics {
  renderTime: number;
  scrollTime: number;
  memoryUsage: number;
  domNodeCount: number;
}

export class PerformanceThresholdAnalyzer {
  private static readonly RENDER_TIME_THRESHOLD = 2000; // 2 seconds
  private static readonly SCROLL_TIME_THRESHOLD = 500; // 0.5 seconds
  private static readonly MEMORY_GROWTH_THRESHOLD = 1.5; // 1.5x growth rate

  static shouldUseVirtualization(
    itemCount: number,
    metrics: PerformanceMetrics,
  ): boolean {
    // Basic threshold check - below 100 items, virtualization not needed
    if (itemCount < 100) return false;

    // Performance degradation check
    if (metrics.renderTime > PerformanceThresholdAnalyzer.RENDER_TIME_THRESHOLD)
      return true;
    if (metrics.scrollTime > PerformanceThresholdAnalyzer.SCROLL_TIME_THRESHOLD)
      return true;

    // Memory usage check
    const expectedMemoryUsage =
      PerformanceThresholdAnalyzer.calculateExpectedMemory(itemCount);
    if (
      metrics.memoryUsage >
      expectedMemoryUsage * PerformanceThresholdAnalyzer.MEMORY_GROWTH_THRESHOLD
    ) {
      return true;
    }

    // For large item counts, recommend virtualization even if performance is acceptable
    if (itemCount >= 200) return true;

    return false;
  }

  private static calculateExpectedMemory(itemCount: number): number {
    // Empirical calculation: base memory + per-item memory
    const BASE_MEMORY = 1024 * 1024; // 1MB base
    const MEMORY_PER_ITEM = 50 * 1024; // 50KB per item
    return BASE_MEMORY + itemCount * MEMORY_PER_ITEM;
  }

  static analyzePerformanceData(
    results: Array<{
      itemCount: number;
      renderTime: number;
      scrollTime: number;
      memoryUsage: number;
      domNodeCount: number;
    }>,
  ): {
    recommendations: string[];
    thresholds: {
      virtualizationRecommended: number;
      virtualizationRequired: number;
    };
  } {
    const recommendations: string[] = [];

    // Analyze trends
    const sortedResults = results.sort((a, b) => a.itemCount - b.itemCount);

    // Find performance degradation points
    for (let i = 1; i < sortedResults.length; i++) {
      const current = sortedResults[i];
      const previous = sortedResults[i - 1];

      const renderTimeGrowth = current.renderTime / previous.renderTime;
      const memoryGrowth = current.memoryUsage / previous.memoryUsage;

      if (renderTimeGrowth > 1.5) {
        recommendations.push(
          `Render time increases significantly at ${current.itemCount} items (${renderTimeGrowth.toFixed(2)}x)`,
        );
      }

      if (memoryGrowth > 1.5) {
        recommendations.push(
          `Memory usage increases significantly at ${current.itemCount} items (${memoryGrowth.toFixed(2)}x)`,
        );
      }
    }

    // Find recommended thresholds
    let virtualizationRecommended = 200; // Default
    let virtualizationRequired = 500; // Default

    const problemPoint = sortedResults.find(
      (result) =>
        result.renderTime >
          PerformanceThresholdAnalyzer.RENDER_TIME_THRESHOLD ||
        result.scrollTime > PerformanceThresholdAnalyzer.SCROLL_TIME_THRESHOLD,
    );

    if (problemPoint) {
      virtualizationRequired = problemPoint.itemCount;
      virtualizationRecommended = Math.max(
        100,
        Math.floor(problemPoint.itemCount * 0.7),
      );

      recommendations.push(
        `Performance issues detected at ${problemPoint.itemCount} items - virtualization required`,
      );
    }

    return {
      recommendations,
      thresholds: {
        virtualizationRecommended,
        virtualizationRequired,
      },
    };
  }

  static generatePerformanceReport(
    results: Array<{
      itemCount: number;
      renderTime: number;
      scrollTime: number;
      memoryUsage: number;
      domNodeCount: number;
    }>,
  ): string {
    const analysis =
      PerformanceThresholdAnalyzer.analyzePerformanceData(results);

    let report = "Performance Analysis Report\n";
    report += "================================\n\n";

    report += "Test Results:\n";
    results.forEach((result) => {
      report += `${result.itemCount} items: `;
      report += `Render ${result.renderTime}ms, `;
      report += `Scroll ${result.scrollTime}ms, `;
      report += `Memory ${Math.round(result.memoryUsage / 1024 / 1024)}MB, `;
      report += `DOM ${result.domNodeCount} nodes\n`;
    });

    report += "\nRecommendations:\n";
    if (analysis.recommendations.length === 0) {
      report += "- Performance is acceptable across all tested item counts\n";
    } else {
      analysis.recommendations.forEach((rec) => {
        report += `- ${rec}\n`;
      });
    }

    report += "\nThresholds:\n";
    report += `- Virtualization recommended: ${analysis.thresholds.virtualizationRecommended} items\n`;
    report += `- Virtualization required: ${analysis.thresholds.virtualizationRequired} items\n`;

    return report;
  }
}

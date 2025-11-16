import type { Page } from "@playwright/test";

/**
 * Performance testing utilities for Core Web Vitals and metrics
 */

/**
 * Core Web Vitals thresholds (Google's recommended values)
 */
export const WEB_VITALS_THRESHOLDS = {
  // Largest Contentful Paint (LCP) - should be < 2.5s
  LCP: {
    good: 2500,
    needsImprovement: 4000,
  },
  // First Input Delay (FID) - should be < 100ms
  FID: {
    good: 100,
    needsImprovement: 300,
  },
  // Cumulative Layout Shift (CLS) - should be < 0.1
  CLS: {
    good: 0.1,
    needsImprovement: 0.25,
  },
  // First Contentful Paint (FCP) - should be < 1.8s
  FCP: {
    good: 1800,
    needsImprovement: 3000,
  },
  // Time to Interactive (TTI) - should be < 3.8s
  TTI: {
    good: 3800,
    needsImprovement: 7300,
  },
  // Total Blocking Time (TBT) - should be < 200ms
  TBT: {
    good: 200,
    needsImprovement: 600,
  },
} as const;

/**
 * Performance metrics interface
 */
export interface PerformanceMetrics {
  // Core Web Vitals
  lcp?: number;
  fid?: number;
  cls?: number;
  fcp?: number;
  tti?: number;
  tbt?: number;

  // Navigation Timing
  domContentLoaded?: number;
  loadComplete?: number;

  // Custom metrics
  timeToFirstByte?: number;
  resourceLoadTime?: number;

  // Memory
  jsHeapSize?: number;
}

/**
 * Measure Core Web Vitals
 */
export async function measureWebVitals(
  page: Page,
): Promise<PerformanceMetrics> {
  // Wait for page to load
  await page.waitForLoadState("networkidle");

  // Inject web-vitals library
  await page.addScriptTag({
    url: "https://unpkg.com/web-vitals@3/dist/web-vitals.iife.js",
  });

  // Collect metrics
  const metrics = await page.evaluate(() => {
    return new Promise<PerformanceMetrics>((resolve) => {
      const metrics: PerformanceMetrics = {};

      // @ts-expect-error - web-vitals is loaded dynamically
      const { onLCP, onFID, onCLS, onFCP, onTTFB } = window.webVitals;

      let metricsCollected = 0;
      const totalMetrics = 5;

      const checkComplete = () => {
        metricsCollected++;
        if (metricsCollected === totalMetrics) {
          resolve(metrics);
        }
      };

      onLCP((metric: any) => {
        metrics.lcp = metric.value;
        checkComplete();
      });

      onFID((metric: any) => {
        metrics.fid = metric.value;
        checkComplete();
      });

      onCLS((metric: any) => {
        metrics.cls = metric.value;
        checkComplete();
      });

      onFCP((metric: any) => {
        metrics.fcp = metric.value;
        checkComplete();
      });

      onTTFB((metric: any) => {
        metrics.timeToFirstByte = metric.value;
        checkComplete();
      });

      // Fallback timeout
      setTimeout(() => resolve(metrics), 5000);
    });
  });

  return metrics;
}

/**
 * Measure page load performance using Navigation Timing API
 */
export async function measurePageLoad(page: Page): Promise<PerformanceMetrics> {
  await page.waitForLoadState("load");

  return await page.evaluate(() => {
    const perfData = performance.getEntriesByType(
      "navigation",
    )[0] as PerformanceNavigationTiming;

    return {
      domContentLoaded: perfData.domContentLoadedEventEnd - perfData.fetchStart,
      loadComplete: perfData.loadEventEnd - perfData.fetchStart,
      timeToFirstByte: perfData.responseStart - perfData.requestStart,
      resourceLoadTime: perfData.responseEnd - perfData.responseStart,
    };
  });
}

/**
 * Measure JavaScript heap size
 */
export async function measureMemory(page: Page): Promise<number> {
  return await page.evaluate(() => {
    // @ts-expect-error - performance.memory is Chrome-specific
    if (performance.memory) {
      // @ts-expect-error
      return performance.memory.usedJSHeapSize;
    }
    return 0;
  });
}

/**
 * Get resource loading metrics
 */
export async function getResourceMetrics(page: Page) {
  return await page.evaluate(() => {
    const resources = performance.getEntriesByType("resource");

    const metrics = {
      totalResources: resources.length,
      scripts: 0,
      stylesheets: 0,
      images: 0,
      fonts: 0,
      totalSize: 0,
      slowestResource: { name: "", duration: 0 },
    };

    resources.forEach((resource: any) => {
      const type = resource.initiatorType;

      if (type === "script") metrics.scripts++;
      else if (type === "css" || type === "link") metrics.stylesheets++;
      else if (type === "img") metrics.images++;
      else if (type === "font") metrics.fonts++;

      if (resource.transferSize) {
        metrics.totalSize += resource.transferSize;
      }

      if (resource.duration > metrics.slowestResource.duration) {
        metrics.slowestResource = {
          name: resource.name,
          duration: resource.duration,
        };
      }
    });

    return metrics;
  });
}

/**
 * Check if metrics meet Web Vitals thresholds
 */
export function evaluateWebVitals(metrics: PerformanceMetrics): {
  lcp: "good" | "needs-improvement" | "poor";
  fid: "good" | "needs-improvement" | "poor";
  cls: "good" | "needs-improvement" | "poor";
  fcp: "good" | "needs-improvement" | "poor";
} {
  const evaluate = (
    value: number | undefined,
    thresholds: { good: number; needsImprovement: number },
  ) => {
    if (!value) return "poor";
    if (value <= thresholds.good) return "good";
    if (value <= thresholds.needsImprovement) return "needs-improvement";
    return "poor";
  };

  return {
    lcp: evaluate(metrics.lcp, WEB_VITALS_THRESHOLDS.LCP),
    fid: evaluate(metrics.fid, WEB_VITALS_THRESHOLDS.FID),
    cls: evaluate(metrics.cls, WEB_VITALS_THRESHOLDS.CLS),
    fcp: evaluate(metrics.fcp, WEB_VITALS_THRESHOLDS.FCP),
  };
}

/**
 * Assert Web Vitals are within acceptable range
 */
export function assertWebVitals(
  metrics: PerformanceMetrics,
  strictMode = false,
) {
  const thresholds = WEB_VITALS_THRESHOLDS;

  const maxLCP = strictMode
    ? thresholds.LCP.good
    : thresholds.LCP.needsImprovement;
  const maxFID = strictMode
    ? thresholds.FID.good
    : thresholds.FID.needsImprovement;
  const maxCLS = strictMode
    ? thresholds.CLS.good
    : thresholds.CLS.needsImprovement;
  const maxFCP = strictMode
    ? thresholds.FCP.good
    : thresholds.FCP.needsImprovement;

  const errors: string[] = [];

  if (metrics.lcp && metrics.lcp > maxLCP) {
    errors.push(`LCP (${metrics.lcp}ms) exceeds threshold (${maxLCP}ms)`);
  }

  if (metrics.fid && metrics.fid > maxFID) {
    errors.push(`FID (${metrics.fid}ms) exceeds threshold (${maxFID}ms)`);
  }

  if (metrics.cls && metrics.cls > maxCLS) {
    errors.push(`CLS (${metrics.cls}) exceeds threshold (${maxCLS})`);
  }

  if (metrics.fcp && metrics.fcp > maxFCP) {
    errors.push(`FCP (${metrics.fcp}ms) exceeds threshold (${maxFCP}ms)`);
  }

  if (errors.length > 0) {
    throw new Error(`Web Vitals check failed:\n${errors.join("\n")}`);
  }
}

/**
 * Generate performance report
 */
export function generatePerformanceReport(metrics: PerformanceMetrics): string {
  const evaluation = evaluateWebVitals(metrics);

  let report = "=== Performance Report ===\n\n";
  report += "Core Web Vitals:\n";
  report += `  LCP: ${metrics.lcp?.toFixed(0) ?? "N/A"}ms [${evaluation.lcp}]\n`;
  report += `  FID: ${metrics.fid?.toFixed(0) ?? "N/A"}ms [${evaluation.fid}]\n`;
  report += `  CLS: ${metrics.cls?.toFixed(3) ?? "N/A"} [${evaluation.cls}]\n`;
  report += `  FCP: ${metrics.fcp?.toFixed(0) ?? "N/A"}ms [${evaluation.fcp}]\n\n`;

  if (metrics.domContentLoaded) {
    report += "Navigation Timing:\n";
    report += `  DOM Content Loaded: ${metrics.domContentLoaded.toFixed(0)}ms\n`;
    report += `  Load Complete: ${metrics.loadComplete?.toFixed(0) ?? "N/A"}ms\n`;
    report += `  TTFB: ${metrics.timeToFirstByte?.toFixed(0) ?? "N/A"}ms\n\n`;
  }

  if (metrics.jsHeapSize) {
    report += "Memory:\n";
    report += `  JS Heap Size: ${(metrics.jsHeapSize / 1024 / 1024).toFixed(2)}MB\n`;
  }

  return report;
}

/**
 * Monitor continuous performance
 */
export async function monitorPerformance(
  page: Page,
  duration = 5000,
): Promise<PerformanceMetrics[]> {
  const metrics: PerformanceMetrics[] = [];
  const interval = 1000; // Collect metrics every second
  const iterations = duration / interval;

  for (let i = 0; i < iterations; i++) {
    const memory = await measureMemory(page);
    metrics.push({ jsHeapSize: memory });
    await page.waitForTimeout(interval);
  }

  return metrics;
}

/**
 * Compare two performance metrics
 */
export function compareMetrics(
  baseline: PerformanceMetrics,
  current: PerformanceMetrics,
): {
  lcp: number;
  fid: number;
  cls: number;
  fcp: number;
} {
  return {
    lcp: calculateDiff(baseline.lcp, current.lcp),
    fid: calculateDiff(baseline.fid, current.fid),
    cls: calculateDiff(baseline.cls, current.cls),
    fcp: calculateDiff(baseline.fcp, current.fcp),
  };
}

function calculateDiff(
  baseline: number | undefined,
  current: number | undefined,
): number {
  if (!baseline || !current) return 0;
  return ((current - baseline) / baseline) * 100;
}

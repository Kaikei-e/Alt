/**
 * Web Vitals measurement using PerformanceObserver
 */
import type { Page } from "@astral/astral";
import { DEFAULT_THRESHOLDS } from "../config/schema.ts";
import { debug } from "../utils/logger.ts";

export type VitalRating = "good" | "needs-improvement" | "poor";

export interface VitalMetric {
  value: number;
  rating: VitalRating;
}

export interface WebVitalsResult {
  lcp: VitalMetric;
  inp: VitalMetric;
  cls: VitalMetric;
  fcp: VitalMetric;
  ttfb: VitalMetric;
  timestamp: number;
}

export interface NavigationTimingResult {
  domContentLoaded: number;
  load: number;
  firstByte: number;
  domInteractive: number;
  resourceCount: number;
}

// Core Web Vitals thresholds (2025)
const VITALS_THRESHOLDS = DEFAULT_THRESHOLDS.vitals;

// JavaScript to inject for collecting Web Vitals
const WEB_VITALS_COLLECTOR_SCRIPT = `
(function() {
  window.__WEB_VITALS__ = {
    lcp: null,
    inp: null,
    cls: 0,
    fcp: null,
    ttfb: null,
    ready: false
  };

  // Largest Contentful Paint
  try {
    const lcpObserver = new PerformanceObserver((entryList) => {
      const entries = entryList.getEntries();
      const lastEntry = entries[entries.length - 1];
      if (lastEntry) {
        window.__WEB_VITALS__.lcp = lastEntry.startTime;
      }
    });
    lcpObserver.observe({ type: 'largest-contentful-paint', buffered: true });
  } catch (e) {
    console.warn('LCP not supported:', e);
  }

  // First Contentful Paint
  try {
    const fcpObserver = new PerformanceObserver((entryList) => {
      const entries = entryList.getEntriesByName('first-contentful-paint');
      if (entries.length > 0) {
        window.__WEB_VITALS__.fcp = entries[0].startTime;
      }
    });
    fcpObserver.observe({ type: 'paint', buffered: true });
  } catch (e) {
    console.warn('FCP not supported:', e);
  }

  // Cumulative Layout Shift
  try {
    let clsValue = 0;
    const clsObserver = new PerformanceObserver((entryList) => {
      for (const entry of entryList.getEntries()) {
        if (!entry.hadRecentInput) {
          clsValue += entry.value;
        }
      }
      window.__WEB_VITALS__.cls = clsValue;
    });
    clsObserver.observe({ type: 'layout-shift', buffered: true });
  } catch (e) {
    console.warn('CLS not supported:', e);
  }

  // Interaction to Next Paint (simplified)
  try {
    const inpObserver = new PerformanceObserver((entryList) => {
      for (const entry of entryList.getEntries()) {
        const inp = entry.processingStart - entry.startTime + entry.duration;
        if (!window.__WEB_VITALS__.inp || inp > window.__WEB_VITALS__.inp) {
          window.__WEB_VITALS__.inp = inp;
        }
      }
    });
    inpObserver.observe({ type: 'event', buffered: true, durationThreshold: 16 });
  } catch (e) {
    console.warn('INP not supported:', e);
  }

  // Time to First Byte from Navigation Timing
  try {
    const navEntries = performance.getEntriesByType('navigation');
    if (navEntries.length > 0) {
      window.__WEB_VITALS__.ttfb = navEntries[0].responseStart;
    }
  } catch (e) {
    console.warn('TTFB not supported:', e);
  }

  window.__WEB_VITALS__.ready = true;
})();
`;

// Get rating based on thresholds
function getRating(
  value: number | null,
  thresholds: { good: number; poor: number }
): VitalRating {
  if (value === null || value === 0) return "needs-improvement";
  if (value <= thresholds.good) return "good";
  if (value <= thresholds.poor) return "needs-improvement";
  return "poor";
}

/**
 * Web Vitals collector
 */
export class WebVitalsCollector {
  private stabilizationDelay: number;

  constructor(options: { stabilizationDelay?: number } = {}) {
    this.stabilizationDelay = options.stabilizationDelay ?? 1500;
  }

  /**
   * Inject Web Vitals collector script into page
   */
  async inject(page: Page): Promise<void> {
    debug("Injecting Web Vitals collector");
    await page.evaluate(WEB_VITALS_COLLECTOR_SCRIPT);
  }

  /**
   * Collect Web Vitals after stabilization period
   */
  async collect(page: Page): Promise<WebVitalsResult> {
    // Wait for metrics to stabilize
    await new Promise((resolve) => setTimeout(resolve, this.stabilizationDelay));

    debug("Collecting Web Vitals");

    const vitals = await page.evaluate(() => {
      const wv = (globalThis as unknown as { __WEB_VITALS__: Record<string, number | null | boolean> }).__WEB_VITALS__;
      return {
        lcp: wv.lcp as number | null,
        inp: wv.inp as number | null,
        cls: wv.cls as number,
        fcp: wv.fcp as number | null,
        ttfb: wv.ttfb as number | null,
      };
    });

    return {
      lcp: {
        value: vitals.lcp ?? 0,
        rating: getRating(vitals.lcp, VITALS_THRESHOLDS.lcp),
      },
      inp: {
        value: vitals.inp ?? 0,
        rating: getRating(vitals.inp, VITALS_THRESHOLDS.inp),
      },
      cls: {
        value: vitals.cls ?? 0,
        rating: getRating(vitals.cls, VITALS_THRESHOLDS.cls),
      },
      fcp: {
        value: vitals.fcp ?? 0,
        rating: getRating(vitals.fcp, VITALS_THRESHOLDS.fcp),
      },
      ttfb: {
        value: vitals.ttfb ?? 0,
        rating: getRating(vitals.ttfb, VITALS_THRESHOLDS.ttfb),
      },
      timestamp: Date.now(),
    };
  }

  /**
   * Collect navigation timing metrics
   */
  async collectNavigationTiming(page: Page): Promise<NavigationTimingResult> {
    debug("Collecting navigation timing");

    const timing = await page.evaluate(`
      (function() {
        const navEntries = performance.getEntriesByType("navigation");
        const navEntry = navEntries.length > 0 ? navEntries[0] : null;
        const resources = performance.getEntriesByType("resource");

        return {
          domContentLoaded: navEntry ? navEntry.domContentLoadedEventEnd : 0,
          load: navEntry ? navEntry.duration : 0,
          firstByte: navEntry ? navEntry.responseStart : 0,
          domInteractive: navEntry ? navEntry.domInteractive : 0,
          resourceCount: resources.length,
        };
      })()
    `) as NavigationTimingResult;

    return timing;
  }
}

/**
 * Create a configured Web Vitals collector
 */
export function createWebVitalsCollector(
  options?: { stabilizationDelay?: number }
): WebVitalsCollector {
  return new WebVitalsCollector(options);
}

/**
 * Calculate overall performance score (0-100)
 */
export function calculateScore(
  vitals: WebVitalsResult,
  weights = DEFAULT_THRESHOLDS.scoring.weights
): number {
  const ratingScores: Record<VitalRating, number> = {
    good: 100,
    "needs-improvement": 50,
    poor: 0,
  };

  const totalWeight = Object.values(weights).reduce((a, b) => a + b, 0);

  const weightedScore =
    (ratingScores[vitals.lcp.rating] * weights.lcp +
      ratingScores[vitals.inp.rating] * weights.inp +
      ratingScores[vitals.cls.rating] * weights.cls +
      ratingScores[vitals.fcp.rating] * weights.fcp +
      ratingScores[vitals.ttfb.rating] * weights.ttfb) /
    totalWeight;

  return Math.round(weightedScore);
}

/**
 * Identify performance bottlenecks
 */
export function identifyBottlenecks(vitals: WebVitalsResult): string[] {
  const bottlenecks: string[] = [];

  if (vitals.lcp.rating === "poor") {
    bottlenecks.push("Large Contentful Paint is poor - check largest element loading");
  }
  if (vitals.inp.rating === "poor") {
    bottlenecks.push("Interaction to Next Paint is poor - check event handlers");
  }
  if (vitals.cls.rating === "poor") {
    bottlenecks.push("Cumulative Layout Shift is poor - check element dimensions");
  }
  if (vitals.fcp.rating === "poor") {
    bottlenecks.push("First Contentful Paint is poor - check render-blocking resources");
  }
  if (vitals.ttfb.rating === "poor") {
    bottlenecks.push("Time to First Byte is poor - check server response time");
  }

  return bottlenecks;
}

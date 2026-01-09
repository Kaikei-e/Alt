/**
 * Scan command - measure Web Vitals for all configured routes
 */
import type { PerfConfig, RouteConfig } from "../config/schema.ts";
import { createBrowserManager } from "../browser/astral.ts";
import { getAuthCookies } from "../auth/kratos-session.ts";
import {
  createWebVitalsCollector,
  calculateScore,
  identifyBottlenecks,
} from "../measurement/vitals.ts";
import type {
  PerformanceReport,
  RouteMeasurement,
  ReportSummary,
} from "../report/types.ts";
import { printCliReport, printCompactSummary } from "../report/cli-reporter.ts";
import { saveJsonReport, printJsonReport } from "../report/json-reporter.ts";
import { printMarkdownReport, saveMarkdownReport } from "../report/markdown-reporter.ts";
import { info, error, warn, progress, section, debug } from "../utils/logger.ts";
import { DEFAULT_THRESHOLDS } from "../config/schema.ts";
import { calculateMedian, discardOutliers, calculateStats } from "../utils/stats.ts";

export type ReportFormat = "cli" | "json" | "md" | "markdown";

interface ScanOptions {
  device?: string;
  route?: string;
  output?: string;
  json?: boolean;
  format?: ReportFormat | string;
  headless?: boolean;
  verbose?: boolean;
  warmup?: number;
  runs?: number;
}

/**
 * Flatten routes from config into a single array
 */
function flattenRoutes(config: PerfConfig): RouteConfig[] {
  const routes: RouteConfig[] = [];
  const routeGroups = config.routes;

  if (routeGroups.public) routes.push(...routeGroups.public);
  if (routeGroups.desktop) routes.push(...routeGroups.desktop);
  if (routeGroups.mobile) routes.push(...routeGroups.mobile);
  if (routeGroups.sveltekit) routes.push(...routeGroups.sveltekit);
  // Skip API routes for scan (they don't have pages)

  return routes;
}

/**
 * Filter routes by path pattern
 */
function filterRoutes(routes: RouteConfig[], pattern?: string): RouteConfig[] {
  if (!pattern) return routes;
  return routes.filter((r) => r.path.includes(pattern));
}

/**
 * Generate recommendations based on results
 */
function generateRecommendations(routes: RouteMeasurement[]): string[] {
  const recommendations: string[] = [];

  // Only consider measured routes (exclude skipped)
  const measuredRoutes = routes.filter((r) => r.status !== "skipped");

  const poorLcp = measuredRoutes.filter((r) => r.vitals.lcp.rating === "poor");
  const poorCls = measuredRoutes.filter((r) => r.vitals.cls.rating === "poor");
  const poorInp = measuredRoutes.filter((r) => r.vitals.inp.rating === "poor");

  // Deduplicate paths (same route may appear with different devices)
  const uniquePaths = (routes: RouteMeasurement[]): string[] => {
    return [...new Set(routes.map((r) => r.path))];
  };

  if (poorLcp.length > 0) {
    const paths = uniquePaths(poorLcp);
    recommendations.push(
      `Optimize LCP on ${paths.length} route(s): ${paths.join(", ")}`
    );
  }
  if (poorCls.length > 0) {
    const paths = uniquePaths(poorCls);
    recommendations.push(
      `Fix layout shifts on ${paths.length} route(s): ${paths.join(", ")}`
    );
  }
  if (poorInp.length > 0) {
    const paths = uniquePaths(poorInp);
    recommendations.push(
      `Improve interactivity on ${paths.length} route(s): ${paths.join(", ")}`
    );
  }

  return recommendations;
}

/**
 * Run scan command
 */
export async function runScan(config: PerfConfig, options: ScanOptions): Promise<void> {
  const startTime = performance.now();
  section("Performance Scan");

  // Get routes to scan
  let routes = flattenRoutes(config);
  routes = filterRoutes(routes, options.route);

  if (routes.length === 0) {
    error("No routes to scan");
    return;
  }

  // Determine devices to test
  const devices = options.device ? [options.device] : config.devices;
  const totalTests = routes.length * devices.length;

  info(`Scanning ${routes.length} routes with ${devices.length} device(s)`);

  // Get auth cookies if needed
  const hasAuthRoutes = routes.some((r) => r.requiresAuth);
  let authCookies: Awaited<ReturnType<typeof getAuthCookies>> = [];

  if (hasAuthRoutes && config.auth.enabled) {
    try {
      authCookies = await getAuthCookies(config.auth);
    } catch (err) {
      error("Authentication failed", { error: String(err) });
      // Continue without auth for public routes
    }
  }

  // Create browser manager
  const browser = createBrowserManager({
    headless: options.headless ?? true,
  });

  const vitalsCollector = createWebVitalsCollector();
  const measurements: RouteMeasurement[] = [];

  let completed = 0;

  // Restart browser every N pages to prevent memory issues
  const BROWSER_RESTART_INTERVAL = 20;
  let pageCount = 0;

  // Configuration for multi-run measurement
  const warmupRuns = options.warmup ?? 1;
  const measurementRuns = options.runs ?? 1;

  try {
    await browser.launch();

    // Warmup phase - visit routes to warm up server/browser caches
    if (warmupRuns > 0 && authCookies.length > 0) {
      section("Warmup Phase");
      info(`Running ${warmupRuns} warmup run(s) to stabilize measurements...`);

      // Select a subset of routes for warmup (first authenticated route from each type)
      const warmupRoutes = routes.filter((r) => r.requiresAuth).slice(0, 3);

      for (let run = 0; run < warmupRuns; run++) {
        for (const device of devices) {
          for (const route of warmupRoutes) {
            let page = null;
            try {
              page = await browser.createPage(device, authCookies);
              const url = `${config.baseUrl}${route.path}`;
              await browser.navigateTo(page, url, { waitFor: route.waitFor, timeout: 30000 });
              debug(`Warmup: ${device} ${route.path}`);
            } catch {
              // Ignore warmup errors
            } finally {
              if (page) {
                try {
                  await page.close();
                } catch {
                  // Ignore close errors
                }
              }
            }
          }
        }
      }
      info("Warmup complete");
    }

    section("Measurement Phase");

    for (const device of devices) {
      for (const route of routes) {
        completed++;
        progress(completed, totalTests, `${device}: ${route.path}`);

        // Restart browser periodically to prevent memory issues
        pageCount++;
        if (pageCount > BROWSER_RESTART_INTERVAL) {
          info("Restarting browser to free memory...");
          await browser.close();
          await browser.launch();
          pageCount = 0;
        }

        // Skip auth routes if not authenticated
        if (route.requiresAuth && authCookies.length === 0) {
          measurements.push({
            path: route.path,
            name: route.name,
            device,
            requiresAuth: true,
            vitals: {
              lcp: { value: 0, rating: "good" },
              inp: { value: 0, rating: "good" },
              cls: { value: 0, rating: "good" },
              fcp: { value: 0, rating: "good" },
              ttfb: { value: 0, rating: "good" },
              timestamp: Date.now(),
            },
            timing: {
              domContentLoaded: 0,
              load: 0,
              firstByte: 0,
              domInteractive: 0,
              resourceCount: 0,
            },
            score: 0,
            passed: false,
            status: "skipped",
            skipReason: "Authentication credentials not configured (PERF_TEST_EMAIL/PERF_TEST_PASSWORD)",
            bottlenecks: [],
          });
          continue;
        }

        try {
          // Multi-run measurement for statistical accuracy
          const runVitals: Array<Awaited<ReturnType<typeof vitalsCollector.collect>>> = [];
          const runTimings: Array<Awaited<ReturnType<typeof vitalsCollector.collectNavigationTiming>>> = [];

          for (let run = 0; run < measurementRuns; run++) {
            let page = null;
            try {
              // Create page with cookies
              const cookies = route.requiresAuth ? authCookies : [];
              page = await browser.createPage(device, cookies);

              // Navigate to URL with timeout
              const url = `${config.baseUrl}${route.path}`;
              await browser.navigateTo(page, url, { waitFor: route.waitFor, timeout: 30000 });

              // Inject and collect vitals
              await vitalsCollector.inject(page);
              const vitals = await vitalsCollector.collect(page);
              const timing = await vitalsCollector.collectNavigationTiming(page);

              runVitals.push(vitals);
              runTimings.push(timing);

              if (measurementRuns > 1) {
                debug(`Run ${run + 1}/${measurementRuns}: LCP=${vitals.lcp.value}ms, TTFB=${vitals.ttfb.value}ms`);
              }
            } finally {
              if (page) {
                try {
                  await page.close();
                } catch {
                  // Ignore close errors
                }
              }
            }
          }

          // Aggregate results using median (more robust than mean for performance data)
          const aggregateVitals = (metric: "lcp" | "inp" | "cls" | "fcp" | "ttfb") => {
            const values = runVitals.map((v) => v[metric].value);
            const cleanValues = measurementRuns >= 3 ? discardOutliers(values) : values;
            return calculateMedian(cleanValues);
          };

          const aggregateTiming = (key: keyof typeof runTimings[0]) => {
            const values = runTimings.map((t) => t[key] as number);
            const cleanValues = measurementRuns >= 3 ? discardOutliers(values) : values;
            return calculateMedian(cleanValues);
          };

          // Use median values for final vitals
          const lastVitals = runVitals[runVitals.length - 1];
          const vitals = {
            lcp: { value: aggregateVitals("lcp"), rating: lastVitals.lcp.rating },
            inp: { value: aggregateVitals("inp"), rating: lastVitals.inp.rating },
            cls: { value: aggregateVitals("cls"), rating: lastVitals.cls.rating },
            fcp: { value: aggregateVitals("fcp"), rating: lastVitals.fcp.rating },
            ttfb: { value: aggregateVitals("ttfb"), rating: lastVitals.ttfb.rating },
            timestamp: Date.now(),
          };

          // Re-calculate ratings based on aggregated values
          const getRating = (value: number, good: number, poor: number): "good" | "needs-improvement" | "poor" => {
            if (value <= good) return "good";
            if (value <= poor) return "needs-improvement";
            return "poor";
          };

          vitals.lcp.rating = getRating(vitals.lcp.value, DEFAULT_THRESHOLDS.vitals.lcp.good, DEFAULT_THRESHOLDS.vitals.lcp.poor);
          vitals.inp.rating = getRating(vitals.inp.value, DEFAULT_THRESHOLDS.vitals.inp.good, DEFAULT_THRESHOLDS.vitals.inp.poor);
          vitals.cls.rating = getRating(vitals.cls.value, DEFAULT_THRESHOLDS.vitals.cls.good, DEFAULT_THRESHOLDS.vitals.cls.poor);
          vitals.fcp.rating = getRating(vitals.fcp.value, DEFAULT_THRESHOLDS.vitals.fcp.good, DEFAULT_THRESHOLDS.vitals.fcp.poor);
          vitals.ttfb.rating = getRating(vitals.ttfb.value, DEFAULT_THRESHOLDS.vitals.ttfb.good, DEFAULT_THRESHOLDS.vitals.ttfb.poor);

          const timing = {
            domContentLoaded: aggregateTiming("domContentLoaded"),
            load: aggregateTiming("load"),
            firstByte: aggregateTiming("firstByte"),
            domInteractive: aggregateTiming("domInteractive"),
            resourceCount: Math.round(aggregateTiming("resourceCount")),
          };

          // Calculate score and identify bottlenecks
          const score = calculateScore(vitals);
          const bottlenecks = identifyBottlenecks(vitals);
          const passed = score >= DEFAULT_THRESHOLDS.scoring.passThreshold;

          measurements.push({
            path: route.path,
            name: route.name,
            device,
            requiresAuth: route.requiresAuth ?? false,
            vitals,
            timing,
            score,
            passed,
            status: passed ? "passed" : "failed",
            bottlenecks,
          });
        } catch (err) {
          warn(`Route measurement failed: ${route.path}`, { error: String(err), device });
          measurements.push({
            path: route.path,
            name: route.name,
            device,
            requiresAuth: route.requiresAuth ?? false,
            vitals: {
              lcp: { value: 0, rating: "poor" },
              inp: { value: 0, rating: "poor" },
              cls: { value: 0, rating: "poor" },
              fcp: { value: 0, rating: "poor" },
              ttfb: { value: 0, rating: "poor" },
              timestamp: Date.now(),
            },
            timing: {
              domContentLoaded: 0,
              load: 0,
              firstByte: 0,
              domInteractive: 0,
              resourceCount: 0,
            },
            score: 0,
            passed: false,
            status: "failed",
            bottlenecks: [],
            error: String(err),
          });
        }
      }
    }
  } finally {
    await browser.close();
  }

  // Calculate summary (excluding skipped routes from score calculation)
  const skippedRoutes = measurements.filter((m) => m.status === "skipped").length;
  const measuredRoutes = measurements.filter((m) => m.status !== "skipped");
  const passedRoutes = measuredRoutes.filter((m) => m.passed).length;
  const failedRoutes = measuredRoutes.filter((m) => !m.passed).length;
  const overallScore =
    measuredRoutes.length > 0
      ? Math.round(measuredRoutes.reduce((sum, m) => sum + m.score, 0) / measuredRoutes.length)
      : 0;

  const summary: ReportSummary = {
    totalRoutes: measurements.length,
    passedRoutes,
    failedRoutes,
    skippedRoutes,
    measuredRoutes: measuredRoutes.length,
    overallScore,
    overallRating:
      overallScore >= 90 ? "good" : overallScore >= 50 ? "needs-improvement" : "poor",
  };

  // Generate report
  const report: PerformanceReport = {
    metadata: {
      timestamp: new Date().toISOString(),
      duration: performance.now() - startTime,
      toolVersion: "1.0.0",
      baseUrl: config.baseUrl,
      devices,
    },
    summary,
    routes: measurements,
    recommendations: generateRecommendations(measurements),
  };

  // Determine output format (--format takes precedence over --json)
  const format = options.format || (options.json ? "json" : "cli");

  // Output report
  switch (format) {
    case "json":
      printJsonReport(report);
      break;
    case "md":
    case "markdown":
      printMarkdownReport(report);
      break;
    case "cli":
    default:
      printCliReport(report);
      break;
  }

  // Auto-generate output path if not specified
  const timestamp = new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19);
  let outputPath = options.output;

  if (!outputPath) {
    // Auto-save to reports directory
    const ext = (format === "md" || format === "markdown") ? "md" : "json";
    outputPath = `./reports/scan-${timestamp}.${ext}`;
  }

  // Ensure reports directory exists
  try {
    await Deno.mkdir("./reports", { recursive: true });
  } catch {
    // Directory already exists
  }

  // Save report
  if (format === "md" || format === "markdown" || outputPath.endsWith(".md")) {
    await saveMarkdownReport(report, outputPath);
  } else {
    await saveJsonReport(report, outputPath);
  }
  info(`Report saved to ${outputPath}`);
}

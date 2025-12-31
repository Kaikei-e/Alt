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
import { info, error, progress, section } from "../utils/logger.ts";
import { DEFAULT_THRESHOLDS } from "../config/schema.ts";

interface ScanOptions {
  device?: string;
  route?: string;
  output?: string;
  json?: boolean;
  headless?: boolean;
  verbose?: boolean;
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
  const poorLcp = routes.filter((r) => r.vitals.lcp.rating === "poor");
  const poorCls = routes.filter((r) => r.vitals.cls.rating === "poor");
  const poorInp = routes.filter((r) => r.vitals.inp.rating === "poor");

  if (poorLcp.length > 0) {
    recommendations.push(
      `Optimize LCP on ${poorLcp.length} route(s): ${poorLcp.map((r) => r.path).join(", ")}`
    );
  }
  if (poorCls.length > 0) {
    recommendations.push(
      `Fix layout shifts on ${poorCls.length} route(s): ${poorCls.map((r) => r.path).join(", ")}`
    );
  }
  if (poorInp.length > 0) {
    recommendations.push(
      `Improve interactivity on ${poorInp.length} route(s): ${poorInp.map((r) => r.path).join(", ")}`
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

  try {
    await browser.launch();

    for (const device of devices) {
      for (const route of routes) {
        completed++;
        progress(completed, totalTests, `${device}: ${route.path}`);

        // Skip auth routes if not authenticated
        if (route.requiresAuth && authCookies.length === 0) {
          measurements.push({
            path: route.path,
            name: route.name,
            device,
            requiresAuth: true,
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
            bottlenecks: [],
            error: "Authentication required but not available",
          });
          continue;
        }

        try {
          // Create page with cookies
          const cookies = route.requiresAuth ? authCookies : [];
          const page = await browser.createPage(device, cookies);

          // Navigate to URL
          const url = `${config.baseUrl}${route.path}`;
          await browser.navigateTo(page, url, { waitFor: route.waitFor });

          // Inject and collect vitals
          await vitalsCollector.inject(page);
          const vitals = await vitalsCollector.collect(page);
          const timing = await vitalsCollector.collectNavigationTiming(page);

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
            bottlenecks,
          });

          await page.close();
        } catch (err) {
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
            bottlenecks: [],
            error: String(err),
          });
        }
      }
    }
  } finally {
    await browser.close();
  }

  // Calculate summary
  const passedRoutes = measurements.filter((m) => m.passed).length;
  const overallScore =
    measurements.length > 0
      ? Math.round(measurements.reduce((sum, m) => sum + m.score, 0) / measurements.length)
      : 0;

  const summary: ReportSummary = {
    totalRoutes: measurements.length,
    passedRoutes,
    failedRoutes: measurements.length - passedRoutes,
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

  // Output report
  if (options.json) {
    printJsonReport(report);
  } else {
    printCliReport(report);
  }

  // Save to file if specified
  if (options.output) {
    await saveJsonReport(report, options.output);
    info(`Report saved to ${options.output}`);
  }
}

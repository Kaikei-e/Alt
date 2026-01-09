/**
 * CLI report generator with colored output
 */
import type { PerformanceReport, RouteMeasurement, FlowResult, LoadTestResult, RouteStatus } from "./types.ts";
import type { VitalRating } from "../measurement/vitals.ts";
import {
  bold,
  dim,
  green,
  red,
  yellow,
  cyan,
  horizontalLine,
  ratingColor,
  scoreColor,
  SYMBOLS,
} from "../utils/colors.ts";

/**
 * Get status color function
 */
function statusColor(status: RouteStatus): (text: string) => string {
  switch (status) {
    case "passed":
      return green;
    case "failed":
      return red;
    case "skipped":
      return yellow;
  }
}

/**
 * Get status label
 */
function statusLabel(status: RouteStatus): string {
  switch (status) {
    case "passed":
      return "PASS";
    case "failed":
      return "FAIL";
    case "skipped":
      return "SKIP";
  }
}

/**
 * Format duration in human-readable form
 */
function formatDuration(ms: number): string {
  if (ms >= 1000) {
    return `${(ms / 1000).toFixed(2)}s`;
  }
  return `${Math.round(ms)}ms`;
}

/**
 * Format metric value with unit and rating color
 */
function formatMetric(value: number, rating: VitalRating, unit: string = "ms"): string {
  const colorFn = ratingColor(rating);
  const displayValue = unit === "ms" ? Math.round(value) : value.toFixed(3);
  return colorFn(`${displayValue}${unit === "ms" ? "ms" : ""}`);
}

/**
 * Print report header
 */
function printHeader(): void {
  console.log("");
  console.log(bold("=== Alt Performance Report ==="));
  console.log("");
}

/**
 * Print summary section
 */
function printSummary(report: PerformanceReport): void {
  const { summary, metadata } = report;
  const colorFn = scoreColor(summary.overallScore);

  console.log(bold("Summary:"));
  console.log(`  Base URL: ${cyan(metadata.baseUrl)}`);
  console.log(`  Duration: ${formatDuration(metadata.duration)}`);
  console.log(`  Devices: ${metadata.devices.join(", ")}`);
  console.log("");
  console.log(`  Total Routes: ${summary.totalRoutes}`);
  console.log(`  Measured: ${summary.measuredRoutes}`);
  console.log(`  Passed: ${green(summary.passedRoutes.toString())}`);
  console.log(`  Failed: ${red(summary.failedRoutes.toString())}`);
  if (summary.skippedRoutes > 0) {
    console.log(`  Skipped: ${yellow(summary.skippedRoutes.toString())} (auth routes - no credentials)`);
  }
  console.log(`  Score: ${colorFn(summary.overallScore.toString())}/100 (based on measured routes)`);
  console.log("");
}

/**
 * Print route details
 */
function printRouteDetails(routes: RouteMeasurement[]): void {
  // Separate measured routes and skipped routes
  const measuredRoutes = routes.filter((r) => r.status !== "skipped");
  const skippedRoutes = routes.filter((r) => r.status === "skipped");

  // Print measured routes
  if (measuredRoutes.length > 0) {
    console.log(bold("Measured Routes:"));
    console.log(horizontalLine());

    for (const route of measuredRoutes) {
      const colorFn = statusColor(route.status);
      const label = statusLabel(route.status);
      const authBadge = route.requiresAuth ? dim("[auth]") : "";

      console.log(`${colorFn(label)} ${route.path} ${authBadge}`);
      console.log(`     Device: ${route.device}`);
      console.log(`     LCP: ${formatMetric(route.vitals.lcp.value, route.vitals.lcp.rating)}`);
      console.log(`     INP: ${formatMetric(route.vitals.inp.value, route.vitals.inp.rating)}`);
      console.log(`     CLS: ${formatMetric(route.vitals.cls.value, route.vitals.cls.rating, "")}`);
      console.log(`     FCP: ${formatMetric(route.vitals.fcp.value, route.vitals.fcp.rating)}`);
      console.log(`     TTFB: ${formatMetric(route.vitals.ttfb.value, route.vitals.ttfb.rating)}`);
      console.log(`     Score: ${scoreColor(route.score)(route.score.toString())}/100`);

      if (route.bottlenecks.length > 0) {
        console.log(`     Bottlenecks:`);
        for (const bottleneck of route.bottlenecks) {
          console.log(`       ${yellow(SYMBOLS.bullet)} ${bottleneck}`);
        }
      }

      if (route.error) {
        console.log(`     Error: ${red(route.error)}`);
      }

      console.log(horizontalLine());
    }
  }

  // Print skipped routes summary (collapsed)
  if (skippedRoutes.length > 0) {
    console.log("");
    console.log(bold("Skipped Routes:"));
    console.log(dim(`  (${skippedRoutes.length} routes skipped - authentication credentials not configured)`));
    console.log("");

    // Group by device and list paths compactly
    const byDevice = new Map<string, string[]>();
    for (const route of skippedRoutes) {
      const paths = byDevice.get(route.device) || [];
      paths.push(route.path);
      byDevice.set(route.device, paths);
    }

    for (const [device, paths] of byDevice) {
      console.log(`  ${yellow(device)}:`);
      // Show first few routes, then count
      const maxShow = 5;
      const shown = paths.slice(0, maxShow);
      const remaining = paths.length - maxShow;
      for (const path of shown) {
        console.log(`    ${dim(SYMBOLS.bullet)} ${path}`);
      }
      if (remaining > 0) {
        console.log(`    ${dim(`... and ${remaining} more`)}`);
      }
    }
    console.log("");
    console.log(dim("  To measure these routes, set PERF_TEST_EMAIL and PERF_TEST_PASSWORD environment variables."));
    console.log(horizontalLine());
  }
}

/**
 * Print flow results
 */
function printFlowResults(flows: FlowResult[]): void {
  if (flows.length === 0) return;

  console.log("");
  console.log(bold("User Flow Results:"));
  console.log(horizontalLine());

  for (const flow of flows) {
    const status = flow.passed ? green("PASS") : red("FAIL");
    console.log(`${status} ${flow.name}`);
    console.log(`     Device: ${flow.device}`);
    console.log(`     Duration: ${formatDuration(flow.totalDuration)}`);

    if (flow.steps.length > 0) {
      console.log(`     Steps:`);
      for (const step of flow.steps) {
        const stepStatus = step.success ? green(SYMBOLS.pass) : red(SYMBOLS.fail);
        const target = step.target ? dim(` (${step.target})`) : "";
        console.log(`       ${stepStatus} ${step.action}${target} - ${formatDuration(step.duration)}`);
      }
    }

    if (flow.error) {
      console.log(`     Error: ${red(flow.error)}`);
    }

    console.log(horizontalLine());
  }
}

/**
 * Print load test results
 */
function printLoadTestResults(loadTest: LoadTestResult): void {
  console.log("");
  console.log(bold("Load Test Results:"));
  console.log(horizontalLine());

  console.log(`  URL: ${cyan(loadTest.url)}`);
  console.log(`  Duration: ${formatDuration(loadTest.duration)}`);
  console.log("");
  console.log(`  Total Requests: ${loadTest.totalRequests}`);
  console.log(`  Successful: ${green(loadTest.successfulRequests.toString())}`);
  console.log(`  Failed: ${red(loadTest.failedRequests.toString())}`);
  console.log(`  Error Rate: ${loadTest.errorRate > 0.01 ? red((loadTest.errorRate * 100).toFixed(2) + "%") : green((loadTest.errorRate * 100).toFixed(2) + "%")}`);
  console.log(`  Throughput: ${loadTest.throughput.toFixed(2)} req/s`);
  console.log("");
  console.log(bold("  Response Times:"));
  console.log(`    Min: ${formatDuration(loadTest.responseTimes.min)}`);
  console.log(`    Max: ${formatDuration(loadTest.responseTimes.max)}`);
  console.log(`    Mean: ${formatDuration(loadTest.responseTimes.mean)}`);
  console.log(`    Median: ${formatDuration(loadTest.responseTimes.median)}`);
  console.log(`    p95: ${formatDuration(loadTest.responseTimes.p95)}`);
  console.log(`    p99: ${formatDuration(loadTest.responseTimes.p99)}`);

  if (loadTest.errors.length > 0) {
    console.log("");
    console.log(bold("  Errors by Status:"));
    for (const err of loadTest.errors) {
      const statusLabel = err.status === 0 ? "Network Error" : `HTTP ${err.status}`;
      console.log(`    ${red(statusLabel)}: ${err.count}`);
    }
  }

  console.log(horizontalLine());
}

/**
 * Print recommendations
 */
function printRecommendations(recommendations: string[]): void {
  if (recommendations.length === 0) return;

  console.log("");
  console.log(bold("Recommendations:"));
  for (const rec of recommendations) {
    console.log(`  ${SYMBOLS.bullet} ${rec}`);
  }
}

/**
 * Print full CLI report
 */
export function printCliReport(report: PerformanceReport): void {
  printHeader();
  printSummary(report);
  printRouteDetails(report.routes);

  if (report.flows && report.flows.length > 0) {
    printFlowResults(report.flows);
  }

  if (report.loadTest) {
    printLoadTestResults(report.loadTest);
  }

  printRecommendations(report.recommendations);

  console.log("");
  console.log(dim(`Report generated at ${report.metadata.timestamp}`));
  console.log("");
}

/**
 * Print compact summary for quick status check
 */
export function printCompactSummary(report: PerformanceReport): void {
  const { summary } = report;
  const statusIcon = summary.failedRoutes === 0 ? SYMBOLS.pass : SYMBOLS.fail;
  const scoreColor$ = scoreColor(summary.overallScore);

  console.log("");
  console.log(
    `${statusIcon} ${summary.passedRoutes}/${summary.totalRoutes} routes passed | Score: ${scoreColor$(summary.overallScore.toString())}/100`
  );
  console.log("");
}

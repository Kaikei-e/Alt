/**
 * CLI report generator with colored output
 */
import type { PerformanceReport, RouteMeasurement, FlowResult, LoadTestResult } from "./types.ts";
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
  console.log(`  Passed: ${green(summary.passedRoutes.toString())}`);
  console.log(`  Failed: ${red(summary.failedRoutes.toString())}`);
  console.log(`  Score: ${colorFn(summary.overallScore.toString())}/100`);
  console.log("");
}

/**
 * Print route details
 */
function printRouteDetails(routes: RouteMeasurement[]): void {
  console.log(bold("Route Details:"));
  console.log(horizontalLine());

  for (const route of routes) {
    const status = route.passed ? green("PASS") : red("FAIL");
    const authBadge = route.requiresAuth ? dim("[auth]") : "";

    console.log(`${status} ${route.path} ${authBadge}`);
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

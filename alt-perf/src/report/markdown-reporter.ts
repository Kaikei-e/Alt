/**
 * Markdown report generator
 * Generates clean, readable Markdown reports for performance results
 */
import type { PerformanceReport, RouteMeasurement } from "./types.ts";
import { DEFAULT_THRESHOLDS } from "../config/schema.ts";

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
 * Format metric value
 */
function formatMetric(value: number, unit: string = "ms"): string {
  if (unit === "ms") {
    return `${Math.round(value)}ms`;
  }
  return value.toFixed(3);
}

/**
 * Get status emoji
 */
function statusEmoji(passed: boolean, status?: string): string {
  if (status === "skipped") return "⏭️";
  return passed ? "✅" : "❌";
}

/**
 * Get rating emoji
 */
function ratingEmoji(rating: string): string {
  switch (rating) {
    case "good":
      return "🟢";
    case "needs-improvement":
      return "🟡";
    case "poor":
      return "🔴";
    default:
      return "⚪";
  }
}

/**
 * Generate Markdown report from performance data
 */
export function generateMarkdownReport(report: PerformanceReport): string {
  const lines: string[] = [];
  const { metadata, summary, routes, recommendations } = report;

  // Header
  lines.push("# Alt Performance Report");
  lines.push("");
  lines.push(`**Generated:** ${metadata.timestamp}`);
  lines.push(`**Base URL:** ${metadata.baseUrl}`);
  lines.push(`**Duration:** ${formatDuration(metadata.duration)}`);
  lines.push(`**Devices:** ${metadata.devices.join(", ")}`);
  lines.push("");

  // Summary
  lines.push("## Summary");
  lines.push("");
  lines.push("| Metric | Value |");
  lines.push("|--------|-------|");
  lines.push(`| Total Routes | ${summary.totalRoutes} |`);
  lines.push(`| Measured | ${summary.measuredRoutes} |`);
  lines.push(`| Passed | ${summary.passedRoutes} ✅ |`);
  lines.push(`| Failed | ${summary.failedRoutes} ❌ |`);
  if (summary.skippedRoutes > 0) {
    lines.push(`| Skipped | ${summary.skippedRoutes} ⏭️ |`);
  }
  lines.push(`| Score | **${summary.overallScore}/100** ${ratingEmoji(summary.overallRating)} |`);
  lines.push("");

  // Group routes by device
  const measuredRoutes = routes.filter((r) => r.status !== "skipped");
  const skippedRoutes = routes.filter((r) => r.status === "skipped");
  const devices = [...new Set(measuredRoutes.map((r) => r.device))];

  // Results by Device
  lines.push("## Results by Device");
  lines.push("");

  for (const device of devices) {
    const deviceRoutes = measuredRoutes.filter((r) => r.device === device);

    lines.push(`### ${device}`);
    lines.push("");
    lines.push("| Route | LCP | INP | CLS | FCP | TTFB | Score | Status |");
    lines.push("|-------|-----|-----|-----|-----|------|-------|--------|");

    for (const route of deviceRoutes) {
      const authBadge = route.requiresAuth ? " 🔒" : "";
      const status = statusEmoji(route.passed, route.status);
      lines.push(
        `| ${route.path}${authBadge} | ${formatMetric(route.vitals.lcp.value)} | ${formatMetric(route.vitals.inp.value)} | ${formatMetric(route.vitals.cls.value, "")} | ${formatMetric(route.vitals.fcp.value)} | ${formatMetric(route.vitals.ttfb.value)} | ${route.score} | ${status} |`
      );
    }
    lines.push("");
  }

  // Failed Routes Analysis
  const failedRoutes = measuredRoutes.filter((r) => !r.passed);
  if (failedRoutes.length > 0) {
    lines.push("## Failed Routes Analysis");
    lines.push("");
    lines.push("| Route | Device | Issue | Value | Threshold |");
    lines.push("|-------|--------|-------|-------|-----------|");

    for (const route of failedRoutes) {
      const issues = analyzeIssues(route);
      for (const issue of issues) {
        lines.push(`| ${route.path} | ${route.device} | ${issue.metric} | ${issue.value} | ${issue.threshold} |`);
      }
    }
    lines.push("");
  }

  // Skipped Routes
  if (skippedRoutes.length > 0) {
    lines.push("## Skipped Routes");
    lines.push("");
    lines.push(`> ${skippedRoutes.length} routes were skipped due to missing authentication credentials.`);
    lines.push(">");
    lines.push("> Set `PERF_TEST_EMAIL` and `PERF_TEST_PASSWORD` environment variables to measure these routes.");
    lines.push("");

    // Group by device
    const skippedByDevice = new Map<string, string[]>();
    for (const route of skippedRoutes) {
      const paths = skippedByDevice.get(route.device) || [];
      paths.push(route.path);
      skippedByDevice.set(route.device, paths);
    }

    for (const [device, paths] of skippedByDevice) {
      lines.push(`**${device}:** ${paths.slice(0, 5).join(", ")}${paths.length > 5 ? ` ... and ${paths.length - 5} more` : ""}`);
    }
    lines.push("");
  }

  // Recommendations
  if (recommendations.length > 0) {
    lines.push("## Recommendations");
    lines.push("");
    for (const rec of recommendations) {
      lines.push(`- ${rec}`);
    }
    lines.push("");
  }

  // Core Web Vitals Reference
  lines.push("## Core Web Vitals Thresholds");
  lines.push("");
  lines.push("| Metric | Good | Needs Improvement | Poor |");
  lines.push("|--------|------|-------------------|------|");
  lines.push("| LCP | < 2500ms | 2500-4000ms | > 4000ms |");
  lines.push("| INP | < 200ms | 200-500ms | > 500ms |");
  lines.push("| CLS | < 0.1 | 0.1-0.25 | > 0.25 |");
  lines.push("| FCP | < 1800ms | 1800-3000ms | > 3000ms |");
  lines.push("| TTFB | < 800ms | 800-1800ms | > 1800ms |");
  lines.push("");

  // Footer
  lines.push("---");
  lines.push("");
  lines.push("*Generated by [alt-perf](https://github.com/alt-project/alt-perf)*");

  return lines.join("\n");
}

/**
 * Analyze issues for a failed route
 */
function analyzeIssues(route: RouteMeasurement): Array<{ metric: string; value: string; threshold: string }> {
  const issues: Array<{ metric: string; value: string; threshold: string }> = [];
  const thresholds = DEFAULT_THRESHOLDS.vitals;

  if (route.vitals.lcp.rating === "poor") {
    issues.push({
      metric: "LCP",
      value: formatMetric(route.vitals.lcp.value),
      threshold: `< ${thresholds.lcp.good}ms`,
    });
  }
  if (route.vitals.inp.rating === "poor") {
    issues.push({
      metric: "INP",
      value: formatMetric(route.vitals.inp.value),
      threshold: `< ${thresholds.inp.good}ms`,
    });
  }
  if (route.vitals.cls.rating === "poor") {
    issues.push({
      metric: "CLS",
      value: formatMetric(route.vitals.cls.value, ""),
      threshold: `< ${thresholds.cls.good}`,
    });
  }
  if (route.vitals.fcp.rating === "poor") {
    issues.push({
      metric: "FCP",
      value: formatMetric(route.vitals.fcp.value),
      threshold: `< ${thresholds.fcp.good}ms`,
    });
  }
  if (route.vitals.ttfb.rating === "poor") {
    issues.push({
      metric: "TTFB",
      value: formatMetric(route.vitals.ttfb.value),
      threshold: `< ${thresholds.ttfb.good}ms`,
    });
  }

  // If no poor ratings but still failed (score below threshold)
  if (issues.length === 0) {
    issues.push({
      metric: "Score",
      value: `${route.score}/100`,
      threshold: `>= ${DEFAULT_THRESHOLDS.scoring.passThreshold}`,
    });
  }

  return issues;
}

/**
 * Print Markdown report to console
 */
export function printMarkdownReport(report: PerformanceReport): void {
  console.log(generateMarkdownReport(report));
}

/**
 * Save Markdown report to file
 */
export async function saveMarkdownReport(
  report: PerformanceReport,
  filepath: string
): Promise<void> {
  const content = generateMarkdownReport(report);
  await Deno.writeTextFile(filepath, content);
}

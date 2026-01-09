/**
 * Load command - run load tests against endpoints
 */
import type { PerfConfig, RouteConfig } from "../config/schema.ts";
import { getAuthCookies } from "../auth/kratos-session.ts";
import type { PerformanceReport, LoadTestResult } from "../report/types.ts";
import { printCliReport } from "../report/cli-reporter.ts";
import { saveJsonReport, printJsonReport } from "../report/json-reporter.ts";
import { info, error, progress, section } from "../utils/logger.ts";

interface LoadOptions {
  route?: string;
  duration: number;
  concurrency: number;
  output?: string;
  json?: boolean;
  verbose?: boolean;
}

interface LoadTestConfig {
  url: string;
  method: "GET" | "POST";
  headers?: Record<string, string>;
  concurrency: number;
  duration: number;
  cookies?: Array<{ name: string; value: string }>;
}

/**
 * Run load test against a single URL
 */
async function runLoadTest(config: LoadTestConfig): Promise<LoadTestResult> {
  const results: number[] = [];
  const errors = new Map<number, number>();
  let running = true;

  const startTime = performance.now();
  const endTime = startTime + config.duration * 1000;

  // Create worker function
  const worker = async (): Promise<void> => {
    while (running && performance.now() < endTime) {
      const requestStart = performance.now();

      try {
        const headers = new Headers(config.headers || {});

        if (config.cookies?.length) {
          headers.set(
            "Cookie",
            config.cookies.map((c) => `${c.name}=${c.value}`).join("; ")
          );
        }

        const response = await fetch(config.url, {
          method: config.method,
          headers,
        });

        const responseTime = performance.now() - requestStart;

        if (response.ok) {
          results.push(responseTime);
        } else {
          const count = errors.get(response.status) || 0;
          errors.set(response.status, count + 1);
        }
      } catch {
        const count = errors.get(0) || 0;
        errors.set(0, count + 1);
      }
    }
  };

  // Start workers
  const workers: Promise<void>[] = [];
  for (let i = 0; i < config.concurrency; i++) {
    workers.push(worker());
  }

  // Wait for completion
  await Promise.all(workers);
  running = false;

  const totalDuration = performance.now() - startTime;

  // Calculate statistics
  const sorted = [...results].sort((a, b) => a - b);
  const totalErrors = Array.from(errors.values()).reduce((sum, count) => sum + count, 0);
  const totalRequests = sorted.length + totalErrors;

  return {
    url: config.url,
    totalRequests,
    successfulRequests: sorted.length,
    failedRequests: totalErrors,
    errorRate: totalRequests > 0 ? totalErrors / totalRequests : 0,
    responseTimes: {
      min: sorted[0] || 0,
      max: sorted[sorted.length - 1] || 0,
      mean: sorted.length > 0 ? sorted.reduce((a, b) => a + b, 0) / sorted.length : 0,
      median: sorted[Math.floor(sorted.length / 2)] || 0,
      p95: sorted[Math.floor(sorted.length * 0.95)] || 0,
      p99: sorted[Math.floor(sorted.length * 0.99)] || 0,
    },
    throughput: totalRequests / (totalDuration / 1000),
    duration: totalDuration,
    errors: Array.from(errors.entries()).map(([status, count]) => ({
      status,
      count,
    })),
  };
}

/**
 * Get API routes for load testing
 */
function getLoadTestRoutes(config: PerfConfig, pattern?: string): RouteConfig[] {
  const apiRoutes = config.routes.api || [];

  // Add health endpoints as default
  const defaultRoutes: RouteConfig[] = [
    { path: "/api/health", name: "Frontend Health", type: "api" },
    { path: "/api/backend/v1/health", name: "Backend Health", type: "api" },
  ];

  const routes = [...apiRoutes, ...defaultRoutes].filter(
    (route, index, self) => self.findIndex((r) => r.path === route.path) === index
  );

  if (pattern) {
    return routes.filter((r) => r.path.includes(pattern));
  }

  return routes;
}

/**
 * Run load command
 */
export async function runLoad(config: PerfConfig, options: LoadOptions): Promise<void> {
  const startTime = performance.now();
  section("Load Test");

  // Get routes to test
  const routes = getLoadTestRoutes(config, options.route);

  if (routes.length === 0) {
    error("No routes to test");
    return;
  }

  info(
    `Running load test for ${options.duration}s with ${options.concurrency} concurrent requests`
  );

  // Get auth cookies if any route requires auth
  let authCookies: Awaited<ReturnType<typeof getAuthCookies>> = [];
  if (config.auth.enabled) {
    try {
      authCookies = await getAuthCookies(config.auth);
    } catch {
      // Continue without auth
    }
  }

  const results: LoadTestResult[] = [];

  for (const route of routes) {
    const url = `${config.baseUrl}${route.path}`;
    info(`Testing: ${url}`);

    const result = await runLoadTest({
      url,
      method: "GET",
      concurrency: options.concurrency,
      duration: options.duration,
      cookies: route.requiresAuth ? authCookies : [],
    });

    results.push(result);

    // Print progress
    progress(
      routes.indexOf(route) + 1,
      routes.length,
      `${result.throughput.toFixed(1)} req/s, p95: ${result.responseTimes.p95.toFixed(0)}ms`
    );
  }

  // Generate report
  const report: PerformanceReport = {
    metadata: {
      timestamp: new Date().toISOString(),
      duration: performance.now() - startTime,
      toolVersion: "1.0.0",
      baseUrl: config.baseUrl,
      devices: [],
    },
    summary: {
      totalRoutes: routes.length,
      passedRoutes: results.filter((r) => r.errorRate < 0.01).length,
      failedRoutes: results.filter((r) => r.errorRate >= 0.01).length,
      skippedRoutes: 0,
      measuredRoutes: routes.length,
      overallScore: Math.round(
        (results.filter((r) => r.errorRate < 0.01).length / routes.length) * 100
      ),
      overallRating:
        results.every((r) => r.errorRate < 0.01) ? "good" : "needs-improvement",
    },
    routes: [],
    loadTest: results[0], // First result as primary (or aggregate in future)
    recommendations: [],
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

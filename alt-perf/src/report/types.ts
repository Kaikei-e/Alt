/**
 * Report type definitions
 */
import type { WebVitalsResult, NavigationTimingResult } from "../measurement/vitals.ts";

// Statistical metrics for a single vital
export interface StatisticalMetrics {
  mean: number;
  median: number;
  stdDev: number;
  p95: number;
  p99: number;
  confidenceInterval: {
    lower: number;
    upper: number;
    level: number;
  };
  sampleSize: number;
  isStable: boolean;
}

// Retry information
export interface RetryInfo {
  attempts: number;
  successfulAttempt: number;
  errors: string[];
}

// Route measurement result
export interface RouteMeasurement {
  path: string;
  name: string;
  device: string;
  requiresAuth: boolean;
  vitals: WebVitalsResult;
  timing: NavigationTimingResult;
  score: number;
  passed: boolean;
  bottlenecks: string[];
  error?: string;
  /** Statistical summary for multi-run measurements */
  statistics?: {
    lcp: StatisticalMetrics;
    inp: StatisticalMetrics;
    cls: StatisticalMetrics;
    fcp: StatisticalMetrics;
    ttfb: StatisticalMetrics;
  };
  /** Retry information */
  retryInfo?: RetryInfo;
  /** Debug artifacts */
  artifacts?: {
    screenshot?: string;
    trace?: string;
  };
  /** Network condition used */
  networkCondition?: string;
}

// Flow step result
export interface FlowStepResult {
  action: string;
  target?: string;
  duration: number;
  success: boolean;
  error?: string;
  vitals?: WebVitalsResult;
}

// Flow result
export interface FlowResult {
  name: string;
  description?: string;
  device: string;
  steps: FlowStepResult[];
  totalDuration: number;
  passed: boolean;
  error?: string;
}

// Load test result
export interface LoadTestResult {
  url: string;
  totalRequests: number;
  successfulRequests: number;
  failedRequests: number;
  errorRate: number;
  responseTimes: {
    min: number;
    max: number;
    mean: number;
    median: number;
    p95: number;
    p99: number;
  };
  throughput: number;
  duration: number;
  errors: Array<{ status: number; count: number }>;
}

// Report summary
export interface ReportSummary {
  totalRoutes: number;
  passedRoutes: number;
  failedRoutes: number;
  overallScore: number;
  overallRating: "good" | "needs-improvement" | "poor";
}

// Report metadata
export interface ReportMetadata {
  timestamp: string;
  duration: number;
  toolVersion: string;
  baseUrl: string;
  devices: string[];
}

// Configuration used for the test
export interface ReportConfiguration {
  multiRun: boolean;
  runs?: number;
  retryEnabled: boolean;
  maxAttempts?: number;
  networkCondition?: string;
  cacheDisabled: boolean;
  debugEnabled: boolean;
}

// Reliability metrics for the test run
export interface ReliabilityMetrics {
  totalMeasurements: number;
  successfulMeasurements: number;
  retriedMeasurements: number;
  failedMeasurements: number;
  overallReliability: number;
}

// Artifact information
export interface ArtifactInfo {
  screenshots: string[];
  traces: string[];
  outputDir: string;
}

// Full performance report (enhanced)
export interface PerformanceReport {
  metadata: ReportMetadata;
  summary: ReportSummary;
  routes: RouteMeasurement[];
  flows?: FlowResult[];
  loadTest?: LoadTestResult;
  recommendations: string[];
  /** Configuration used for the test */
  configuration?: ReportConfiguration;
  /** Reliability metrics */
  reliability?: ReliabilityMetrics;
  /** Artifacts generated */
  artifacts?: ArtifactInfo;
}

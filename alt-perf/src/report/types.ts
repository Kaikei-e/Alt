/**
 * Report type definitions
 */
import type { WebVitalsResult, NavigationTimingResult } from "../measurement/vitals.ts";

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

// Full performance report
export interface PerformanceReport {
  metadata: ReportMetadata;
  summary: ReportSummary;
  routes: RouteMeasurement[];
  flows?: FlowResult[];
  loadTest?: LoadTestResult;
  recommendations: string[];
}

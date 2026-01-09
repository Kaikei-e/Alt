/**
 * Configuration schema types for alt-perf
 */

// Device profile for browser emulation
export interface DeviceProfile {
  name: string;
  viewport: { width: number; height: number };
  userAgent: string;
  isMobile: boolean;
  deviceScaleFactor: number;
}

// Predefined device profiles
export const DEVICE_PROFILES: Record<string, DeviceProfile> = {
  "desktop-chrome": {
    name: "Desktop Chrome",
    viewport: { width: 1920, height: 1080 },
    userAgent:
      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
    isMobile: false,
    deviceScaleFactor: 1,
  },
  "mobile-chrome": {
    name: "Mobile Chrome (Pixel 5)",
    viewport: { width: 393, height: 851 },
    userAgent:
      "Mozilla/5.0 (Linux; Android 11; Pixel 5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
    isMobile: true,
    deviceScaleFactor: 2.75,
  },
  "mobile-safari": {
    name: "Mobile Safari (iPhone 12)",
    viewport: { width: 390, height: 844 },
    userAgent:
      "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
    isMobile: true,
    deviceScaleFactor: 3,
  },
};

// Route definition
export interface RouteConfig {
  path: string;
  name: string;
  requiresAuth?: boolean;
  priority?: "high" | "normal" | "low";
  waitFor?: string; // CSS selector to wait for
  type?: "page" | "api";
}

// Route groups
export interface RoutesConfig {
  public?: RouteConfig[];
  desktop?: RouteConfig[];
  mobile?: RouteConfig[];
  sveltekit?: RouteConfig[];
  api?: RouteConfig[];
}

// Auth configuration
export interface AuthConfig {
  enabled: boolean;
  kratosUrl: string;
  authHubUrl: string;
  credentials: {
    email: string;
    password: string;
  };
}

// Threshold configuration
export interface ThresholdConfig {
  good: number;
  poor: number;
}

// Vitals thresholds
export interface VitalsThresholds {
  lcp: ThresholdConfig;
  inp: ThresholdConfig;
  cls: ThresholdConfig;
  fcp: ThresholdConfig;
  ttfb: ThresholdConfig;
}

// Scoring configuration
export interface ScoringConfig {
  passThreshold: number;
  weights: {
    lcp: number;
    inp: number;
    cls: number;
    fcp: number;
    ttfb: number;
  };
}

// Full thresholds config
export interface ThresholdsConfig {
  vitals: VitalsThresholds;
  scoring: ScoringConfig;
}

// Flow step action types
export type FlowAction =
  | "navigate"
  | "click"
  | "fill"
  | "type"
  | "scroll"
  | "swipe"
  | "wait";

// Flow step definition
export interface FlowStep {
  action: FlowAction;
  url?: string;
  selector?: string;
  value?: string;
  direction?: "up" | "down" | "left" | "right";
  amount?: number;
  duration?: number;
  repeat?: number;
  delay?: number;
  waitFor?: string;
  measure?: boolean;
}

// User flow definition
export interface FlowConfig {
  name: string;
  description?: string;
  requiresAuth?: boolean;
  device?: string;
  steps: FlowStep[];
}

// Flows configuration
export interface FlowsConfig {
  flows: FlowConfig[];
}

// Retry configuration
export interface RetryConfig {
  /** Enable retry mechanism */
  enabled: boolean;
  /** Maximum number of retry attempts */
  maxAttempts: number;
  /** Base delay in milliseconds before first retry */
  baseDelayMs: number;
  /** Backoff multiplier for exponential delay */
  backoffMultiplier: number;
}

// Multi-run configuration
export interface MultiRunConfig {
  /** Enable multi-run measurements */
  enabled: boolean;
  /** Number of measurement runs */
  runs: number;
  /** Number of warmup runs (discarded) */
  warmupRuns: number;
  /** Cooldown time between runs in milliseconds */
  cooldownMs: number;
  /** Discard outliers from statistics */
  discardOutliers: boolean;
}

// Debug/artifact configuration
export interface DebugConfig {
  /** Screenshot settings */
  screenshots: {
    /** Enable screenshot capture */
    enabled: boolean;
    /** Capture on test failure */
    onFailure: boolean;
    /** Capture on test success */
    onSuccess: boolean;
    /** Capture full page */
    fullPage: boolean;
  };
  /** Trace settings */
  traces: {
    /** Enable trace capture */
    enabled: boolean;
    /** Capture on test failure */
    onFailure: boolean;
  };
  /** Output directory for artifacts */
  outputDir: string;
  /** Number of days to retain artifacts */
  retentionDays: number;
}

// Network simulation configuration
export interface NetworkSimulationConfig {
  /** Enable network simulation */
  enabled: boolean;
  /** Network condition preset name */
  preset?: string;
  /** Custom network condition */
  custom?: {
    downloadThroughput: number;
    uploadThroughput: number;
    latency: number;
  };
  /** CPU throttling preset */
  cpuThrottling?: string;
}

// Cache configuration
export interface CacheConfig {
  /** Disable browser cache */
  disableCache: boolean;
  /** Clear cache before each test */
  clearBefore: boolean;
}

// Main configuration (enhanced)
export interface PerfConfig {
  baseUrl: string;
  devices: string[];
  auth: AuthConfig;
  routes: RoutesConfig;
  thresholds: ThresholdsConfig;
  flows?: FlowsConfig;
  /** Retry configuration */
  retry?: RetryConfig;
  /** Multi-run configuration */
  multiRun?: MultiRunConfig;
  /** Debug/artifact configuration */
  debug?: DebugConfig;
  /** Network simulation configuration */
  network?: NetworkSimulationConfig;
  /** Cache configuration */
  cache?: CacheConfig;
}

// Default retry configuration
export const DEFAULT_RETRY_CONFIG: RetryConfig = {
  enabled: true,
  maxAttempts: 3,
  baseDelayMs: 1000,
  backoffMultiplier: 2,
};

// Default multi-run configuration
export const DEFAULT_MULTI_RUN_CONFIG: MultiRunConfig = {
  enabled: true,
  runs: 5,
  warmupRuns: 1,
  cooldownMs: 2000,
  discardOutliers: true,
};

// Default debug configuration
export const DEFAULT_DEBUG_CONFIG: DebugConfig = {
  screenshots: {
    enabled: true,
    onFailure: true,
    onSuccess: false,
    fullPage: true,
  },
  traces: {
    enabled: true,
    onFailure: true,
  },
  outputDir: "./artifacts",
  retentionDays: 7,
};

// Default thresholds (Core Web Vitals 2025)
export const DEFAULT_THRESHOLDS: ThresholdsConfig = {
  vitals: {
    lcp: { good: 2500, poor: 4000 },
    inp: { good: 200, poor: 500 },
    cls: { good: 0.1, poor: 0.25 },
    fcp: { good: 1800, poor: 3000 },
    ttfb: { good: 800, poor: 1800 },
  },
  scoring: {
    passThreshold: 80,
    weights: {
      lcp: 25,
      inp: 25,
      cls: 15,
      fcp: 15,
      ttfb: 20,
    },
  },
};

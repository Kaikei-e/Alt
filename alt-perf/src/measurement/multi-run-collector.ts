/**
 * Multi-run measurement orchestrator
 * Collects Web Vitals across multiple runs for statistical reliability
 */
import { BrowserManager, type SessionCookie } from "../browser/astral.ts";
import { WebVitalsCollector, type WebVitalsResult } from "./vitals.ts";
import {
  calculateStatistics,
  type StatisticalSummary,
} from "./statistics.ts";
import { RetryExecutor, DEFAULT_RETRY_POLICY, type RetryPolicy } from "../retry/retry-policy.ts";
import { NetworkController, type NetworkCondition } from "../browser/network-conditions.ts";
import { CacheController, type CacheConfig } from "../browser/cache-controller.ts";
import { debug, info, warn } from "../utils/logger.ts";

/**
 * Multi-run configuration
 */
export interface MultiRunConfig {
  /** Number of measurement runs */
  runs: number;
  /** Number of warmup runs (discarded) */
  warmupRuns: number;
  /** Cooldown time between runs in milliseconds */
  cooldownMs: number;
  /** Discard outliers from statistics */
  discardOutliers: boolean;
  /** Z-score threshold for outlier detection */
  outlierThreshold: number;
}

/**
 * Default multi-run configuration
 */
export const DEFAULT_MULTI_RUN_CONFIG: MultiRunConfig = {
  runs: 5,
  warmupRuns: 1,
  cooldownMs: 2000,
  discardOutliers: true,
  outlierThreshold: 2.5,
};

/**
 * Statistics for a single vital metric across runs
 */
export interface VitalStatistics {
  values: number[];
  summary: StatisticalSummary;
}

/**
 * Result of multi-run measurement
 */
export interface MultiRunResult {
  /** All individual run results */
  runs: WebVitalsResult[];
  /** Statistical summary for each metric */
  statistics: {
    lcp: VitalStatistics;
    inp: VitalStatistics;
    cls: VitalStatistics;
    fcp: VitalStatistics;
    ttfb: VitalStatistics;
  };
  /** Reliability metrics */
  reliability: {
    successfulRuns: number;
    failedRuns: number;
    successRate: number;
    totalAttempts: number;
  };
  /** Configuration used */
  config: MultiRunConfig;
  /** Total duration in milliseconds */
  duration: number;
}

/**
 * Options for multi-run collection
 */
export interface MultiRunOptions {
  /** Session cookies for authentication */
  cookies?: SessionCookie[];
  /** Retry policy for individual runs */
  retryPolicy?: RetryPolicy;
  /** Network condition to simulate */
  networkCondition?: NetworkCondition;
  /** Cache configuration */
  cacheConfig?: CacheConfig;
}

/**
 * Multi-run collector for reliable performance measurements
 */
export class MultiRunCollector {
  private browserManager: BrowserManager;
  private vitalsCollector: WebVitalsCollector;
  private config: MultiRunConfig;
  private networkController: NetworkController;
  private cacheController: CacheController;

  constructor(
    browserManager: BrowserManager,
    vitalsCollector: WebVitalsCollector,
    config: Partial<MultiRunConfig> = {}
  ) {
    this.browserManager = browserManager;
    this.vitalsCollector = vitalsCollector;
    this.config = { ...DEFAULT_MULTI_RUN_CONFIG, ...config };
    this.networkController = new NetworkController();
    this.cacheController = new CacheController();
  }

  /**
   * Collect Web Vitals across multiple runs
   */
  async collectMultiRun(
    url: string,
    device: string,
    options: MultiRunOptions = {}
  ): Promise<MultiRunResult> {
    const startTime = performance.now();
    const retryPolicy = options.retryPolicy || DEFAULT_RETRY_POLICY;
    const retryExecutor = new RetryExecutor<WebVitalsResult>(retryPolicy);

    const runs: WebVitalsResult[] = [];
    let successfulRuns = 0;
    let failedRuns = 0;
    let totalAttempts = 0;

    info(`Starting multi-run collection: ${this.config.runs} runs + ${this.config.warmupRuns} warmup`);

    // Total iterations = warmup + actual runs
    const totalIterations = this.config.warmupRuns + this.config.runs;

    for (let i = 0; i < totalIterations; i++) {
      const isWarmup = i < this.config.warmupRuns;
      const runNumber = isWarmup ? `warmup-${i + 1}` : `run-${i - this.config.warmupRuns + 1}`;

      debug(`Starting ${runNumber}`, { url, device });

      try {
        // Create a fresh page for each run
        const page = await this.browserManager.createPage(device, options.cookies);

        try {
          // Apply network conditions if specified
          if (options.networkCondition) {
            await this.networkController.applyConditions(page, options.networkCondition);
          }

          // Apply cache configuration if specified
          if (options.cacheConfig) {
            await this.cacheController.applyConfig(page, options.cacheConfig);
          }

          // Collect with retry
          const result = await retryExecutor.execute(async () => {
            await this.browserManager.navigateTo(page, url);
            await this.vitalsCollector.inject(page);
            return await this.vitalsCollector.collect(page);
          });

          totalAttempts += result.attempts;

          if (result.success && result.value) {
            if (!isWarmup) {
              runs.push(result.value);
              successfulRuns++;
            }
            debug(`${runNumber} completed`, {
              lcp: result.value.lcp.value,
              attempts: result.attempts,
            });
          } else {
            failedRuns++;
            warn(`${runNumber} failed after ${result.attempts} attempts`);
          }

          // Cleanup cache if configured
          if (options.cacheConfig) {
            await this.cacheController.cleanup(page, options.cacheConfig);
          }

          // Clear throttling
          if (options.networkCondition) {
            await this.networkController.clearConditions(page);
          }
        } finally {
          // Close the page
          await page.close();
        }

        // Cooldown between runs (except after last run)
        if (i < totalIterations - 1) {
          debug(`Cooldown: ${this.config.cooldownMs}ms`);
          await this.sleep(this.config.cooldownMs);
        }
      } catch (error) {
        failedRuns++;
        warn(`${runNumber} failed with error`, { error: String(error) });
      }
    }

    // Calculate statistics
    const statistics = this.calculateVitalStatistics(runs);

    const duration = performance.now() - startTime;

    info(`Multi-run collection completed`, {
      successful: successfulRuns,
      failed: failedRuns,
      duration: `${(duration / 1000).toFixed(1)}s`,
    });

    return {
      runs,
      statistics,
      reliability: {
        successfulRuns,
        failedRuns,
        successRate: successfulRuns / (successfulRuns + failedRuns),
        totalAttempts,
      },
      config: this.config,
      duration,
    };
  }

  /**
   * Calculate statistics for all vital metrics
   */
  private calculateVitalStatistics(
    runs: WebVitalsResult[]
  ): MultiRunResult["statistics"] {
    const extractValues = (
      key: keyof Omit<WebVitalsResult, "timestamp">
    ): number[] => {
      return runs.map((r) => r[key].value);
    };

    const createVitalStatistics = (values: number[]): VitalStatistics => {
      let filteredValues = values;

      // Optionally discard outliers
      if (this.config.discardOutliers && values.length >= 4) {
        const stats = calculateStatistics(values);
        filteredValues = values.filter((v) => !stats.outliers.includes(v));
      }

      return {
        values: filteredValues,
        summary: calculateStatistics(filteredValues),
      };
    };

    return {
      lcp: createVitalStatistics(extractValues("lcp")),
      inp: createVitalStatistics(extractValues("inp")),
      cls: createVitalStatistics(extractValues("cls")),
      fcp: createVitalStatistics(extractValues("fcp")),
      ttfb: createVitalStatistics(extractValues("ttfb")),
    };
  }

  /**
   * Sleep helper
   */
  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  /**
   * Get current configuration
   */
  getConfig(): MultiRunConfig {
    return { ...this.config };
  }

  /**
   * Update configuration
   */
  setConfig(config: Partial<MultiRunConfig>): void {
    this.config = { ...this.config, ...config };
  }
}

/**
 * Create a multi-run collector
 */
export function createMultiRunCollector(
  browserManager: BrowserManager,
  vitalsCollector: WebVitalsCollector,
  config?: Partial<MultiRunConfig>
): MultiRunCollector {
  return new MultiRunCollector(browserManager, vitalsCollector, config);
}

/**
 * Performance trace capture using Chrome DevTools Protocol
 * Provides trace capabilities for detailed performance analysis
 */
import type { Page } from "@astral/astral";
import { ensureDir } from "@std/fs";
import { join } from "@std/path";
import { debug, warn, info } from "../utils/logger.ts";

/**
 * Trace capture options
 */
export interface TraceOptions {
  /** DevTools Protocol tracing categories */
  categories: string[];
  /** Include screenshots in trace */
  screenshots: boolean;
  /** Buffer usage reporting interval in ms */
  bufferUsageReportingInterval?: number;
}

/**
 * Result of a trace capture
 */
export interface TraceResult {
  /** File path where trace was saved */
  path: string;
  /** Trace duration in milliseconds */
  duration: number;
  /** File size in bytes */
  size: number;
  /** Number of trace events */
  events: number;
}

/**
 * Default trace categories for performance analysis
 */
export const DEFAULT_TRACE_CATEGORIES = [
  "devtools.timeline",
  "blink.user_timing",
  "loading",
  "devtools.timeline.async",
  "disabled-by-default-devtools.timeline",
  "disabled-by-default-devtools.timeline.frame",
];

/**
 * Screenshot-enabled trace categories
 */
export const SCREENSHOT_TRACE_CATEGORIES = [
  ...DEFAULT_TRACE_CATEGORIES,
  "disabled-by-default-devtools.screenshot",
];

/**
 * Default trace options
 */
export const DEFAULT_TRACE_OPTIONS: TraceOptions = {
  categories: DEFAULT_TRACE_CATEGORIES,
  screenshots: false,
  bufferUsageReportingInterval: 500,
};

/**
 * Trace capture class using CDP
 */
export class TraceCapture {
  private outputDir: string;
  private traceCount = 0;
  private isTracing = false;
  private traceStartTime = 0;

  constructor(outputDir: string = "./artifacts/traces") {
    this.outputDir = outputDir;
  }

  /**
   * Initialize the output directory
   */
  async initialize(): Promise<void> {
    await ensureDir(this.outputDir);
    debug("Trace output directory initialized", { path: this.outputDir });
  }

  /**
   * Start tracing on the page
   */
  async startTrace(page: Page, options: Partial<TraceOptions> = {}): Promise<void> {
    if (this.isTracing) {
      warn("Trace already in progress");
      return;
    }

    const mergedOptions = { ...DEFAULT_TRACE_OPTIONS, ...options };
    const categories = mergedOptions.screenshots
      ? SCREENSHOT_TRACE_CATEGORIES
      : mergedOptions.categories;

    debug("Starting trace", { categories: categories.join(",") });

    try {
      // Use CDP to start tracing
      const cdpSession = await (page as unknown as { unsafelyGetCDPSession(): Promise<unknown> })
        .unsafelyGetCDPSession?.();

      if (cdpSession) {
        await (cdpSession as { send(method: string, params: unknown): Promise<void> }).send(
          "Tracing.start",
          {
            traceConfig: {
              includedCategories: categories,
              excludedCategories: ["*"],
              syntheticDelays: {},
              memoryDumpConfig: {},
            },
            bufferUsageReportingInterval: mergedOptions.bufferUsageReportingInterval,
          }
        );
      } else {
        // Fallback: use page tracing if available
        debug("CDP session not available, tracing may be limited");
      }

      this.isTracing = true;
      this.traceStartTime = performance.now();
      info("Trace started");
    } catch (error) {
      warn("Failed to start trace", { error: String(error) });
      throw error;
    }
  }

  /**
   * Stop tracing and save the trace file
   */
  async stopTrace(page: Page, name: string): Promise<TraceResult> {
    if (!this.isTracing) {
      throw new Error("No trace in progress");
    }

    this.traceCount++;
    const duration = performance.now() - this.traceStartTime;
    const timestamp = Date.now();

    // Generate filename
    const sanitizedName = name.replace(/[^a-zA-Z0-9-_]/g, "_");
    const filename = `${this.traceCount.toString().padStart(3, "0")}_${sanitizedName}_${timestamp}.json`;
    const filepath = join(this.outputDir, filename);

    debug("Stopping trace", { name, filepath });

    try {
      let events = 0;

      // Use CDP to stop tracing
      const cdpSession = await (page as unknown as { unsafelyGetCDPSession(): Promise<unknown> })
        .unsafelyGetCDPSession?.();

      if (cdpSession) {
        const traceData = await (cdpSession as { send(method: string): Promise<{ value: string }> })
          .send("Tracing.end");

        // Write trace data
        if (traceData && traceData.value) {
          await Deno.writeTextFile(filepath, traceData.value);
          const parsed = JSON.parse(traceData.value);
          events = Array.isArray(parsed.traceEvents) ? parsed.traceEvents.length : 0;
        }
      } else {
        // Create minimal trace file
        const minimalTrace = {
          traceEvents: [],
          metadata: {
            name,
            timestamp,
            duration,
          },
        };
        await Deno.writeTextFile(filepath, JSON.stringify(minimalTrace, null, 2));
      }

      // Get file size
      const stat = await Deno.stat(filepath);

      this.isTracing = false;

      const result: TraceResult = {
        path: filepath,
        duration,
        size: stat.size,
        events,
      };

      info("Trace stopped", {
        path: filepath,
        duration: `${duration.toFixed(0)}ms`,
        size: `${(stat.size / 1024).toFixed(1)}KB`,
      });

      return result;
    } catch (error) {
      this.isTracing = false;
      warn("Failed to stop trace", { name, error: String(error) });
      throw error;
    }
  }

  /**
   * Capture a performance trace around an operation
   */
  async capturePerformanceTrace<T>(
    page: Page,
    operation: () => Promise<T>,
    name: string,
    options?: Partial<TraceOptions>
  ): Promise<{ result: T; trace: TraceResult }> {
    await this.startTrace(page, options);

    try {
      const result = await operation();
      const trace = await this.stopTrace(page, name);
      return { result, trace };
    } catch (error) {
      // Try to stop trace even on error
      try {
        await this.stopTrace(page, `${name}_error`);
      } catch {
        // Ignore stop error
      }
      throw error;
    }
  }

  /**
   * Check if tracing is currently active
   */
  isTracingActive(): boolean {
    return this.isTracing;
  }

  /**
   * Get the number of traces captured
   */
  getTraceCount(): number {
    return this.traceCount;
  }

  /**
   * Get the output directory path
   */
  getOutputDir(): string {
    return this.outputDir;
  }
}

/**
 * Create a trace capture instance
 */
export function createTraceCapture(outputDir?: string): TraceCapture {
  return new TraceCapture(outputDir);
}

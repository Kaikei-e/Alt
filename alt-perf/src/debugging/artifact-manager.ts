/**
 * Centralized artifact management
 * Manages screenshots, traces, and other debugging artifacts
 */
import { ensureDir, walk } from "@std/fs";
import { join } from "@std/path";
import { debug, info, warn } from "../utils/logger.ts";
import { ScreenshotCapture } from "./screenshot.ts";
import { TraceCapture } from "./trace.ts";

/**
 * Artifact configuration
 */
export interface ArtifactConfig {
  /** Base directory for all artifacts */
  baseDir: string;
  /** Number of days to retain artifacts */
  retentionDays: number;
  /** Capture artifacts on failure */
  captureOnFailure: boolean;
  /** Capture artifacts on success */
  captureOnSuccess: boolean;
  /** Enable screenshots */
  screenshotsEnabled: boolean;
  /** Enable traces */
  tracesEnabled: boolean;
}

/**
 * Default artifact configuration
 */
export const DEFAULT_ARTIFACT_CONFIG: ArtifactConfig = {
  baseDir: "./artifacts",
  retentionDays: 7,
  captureOnFailure: true,
  captureOnSuccess: false,
  screenshotsEnabled: true,
  tracesEnabled: true,
};

/**
 * Artifact metadata
 */
export interface ArtifactMetadata {
  /** Type of artifact */
  type: "screenshot" | "trace" | "har" | "log";
  /** File path */
  path: string;
  /** Timestamp when artifact was created */
  timestamp: number;
  /** Test or operation name */
  testName: string;
  /** Route being tested */
  route?: string;
  /** Device profile used */
  device?: string;
  /** Additional context */
  context?: Record<string, string>;
}

/**
 * Artifact manager class
 */
export class ArtifactManager {
  private config: ArtifactConfig;
  private artifacts: ArtifactMetadata[] = [];
  private screenshotCapture: ScreenshotCapture;
  private traceCapture: TraceCapture;
  private initialized = false;

  constructor(config: Partial<ArtifactConfig> = {}) {
    this.config = { ...DEFAULT_ARTIFACT_CONFIG, ...config };
    this.screenshotCapture = new ScreenshotCapture(
      join(this.config.baseDir, "screenshots")
    );
    this.traceCapture = new TraceCapture(join(this.config.baseDir, "traces"));
  }

  /**
   * Initialize the artifact manager
   */
  async initialize(): Promise<void> {
    if (this.initialized) return;

    debug("Initializing artifact manager", { baseDir: this.config.baseDir });

    // Create directory structure
    await ensureDir(this.config.baseDir);
    await ensureDir(join(this.config.baseDir, "screenshots"));
    await ensureDir(join(this.config.baseDir, "traces"));
    await ensureDir(join(this.config.baseDir, "logs"));

    // Initialize sub-managers
    await this.screenshotCapture.initialize();
    await this.traceCapture.initialize();

    // Clean up old artifacts
    await this.cleanup();

    this.initialized = true;
    info("Artifact manager initialized");
  }

  /**
   * Register an artifact
   */
  registerArtifact(metadata: ArtifactMetadata): void {
    this.artifacts.push(metadata);
    debug("Artifact registered", { type: metadata.type, path: metadata.path });
  }

  /**
   * Get all registered artifacts
   */
  getArtifacts(): ArtifactMetadata[] {
    return [...this.artifacts];
  }

  /**
   * Get artifacts by type
   */
  getArtifactsByType(type: ArtifactMetadata["type"]): ArtifactMetadata[] {
    return this.artifacts.filter((a) => a.type === type);
  }

  /**
   * Get the screenshot capture instance
   */
  getScreenshotCapture(): ScreenshotCapture {
    return this.screenshotCapture;
  }

  /**
   * Get the trace capture instance
   */
  getTraceCapture(): TraceCapture {
    return this.traceCapture;
  }

  /**
   * Capture artifacts based on result status
   */
  async captureForResult(
    page: unknown,
    testName: string,
    success: boolean,
    options: { route?: string; device?: string } = {}
  ): Promise<void> {
    const shouldCapture = success
      ? this.config.captureOnSuccess
      : this.config.captureOnFailure;

    if (!shouldCapture) return;

    const suffix = success ? "success" : "failure";

    if (this.config.screenshotsEnabled) {
      try {
        const result = await this.screenshotCapture.capture(
          page as import("@astral/astral").Page,
          `${testName}_${suffix}`
        );
        this.registerArtifact({
          type: "screenshot",
          path: result.path,
          timestamp: result.timestamp,
          testName,
          ...options,
        });
      } catch (error) {
        warn("Failed to capture screenshot for result", { error: String(error) });
      }
    }
  }

  /**
   * Clean up old artifacts based on retention policy
   */
  async cleanup(): Promise<number> {
    const cutoffTime =
      Date.now() - this.config.retentionDays * 24 * 60 * 60 * 1000;
    let removedCount = 0;

    debug("Cleaning up artifacts", {
      retentionDays: this.config.retentionDays,
      cutoffDate: new Date(cutoffTime).toISOString(),
    });

    try {
      for await (const entry of walk(this.config.baseDir, { maxDepth: 3 })) {
        if (!entry.isFile) continue;

        const stat = await Deno.stat(entry.path);
        if (stat.mtime && stat.mtime.getTime() < cutoffTime) {
          await Deno.remove(entry.path);
          removedCount++;
          debug("Removed old artifact", { path: entry.path });
        }
      }

      if (removedCount > 0) {
        info(`Cleaned up ${removedCount} old artifacts`);
      }

      // Also clean up registered artifacts
      this.artifacts = this.artifacts.filter((a) => a.timestamp >= cutoffTime);

      return removedCount;
    } catch (error) {
      warn("Error during artifact cleanup", { error: String(error) });
      return removedCount;
    }
  }

  /**
   * Get summary of artifacts
   */
  getSummary(): {
    total: number;
    byType: Record<string, number>;
    totalSize: number;
  } {
    const byType: Record<string, number> = {};
    const totalSize = 0;

    for (const artifact of this.artifacts) {
      byType[artifact.type] = (byType[artifact.type] || 0) + 1;
    }

    return {
      total: this.artifacts.length,
      byType,
      totalSize,
    };
  }

  /**
   * Write a log artifact
   */
  async writeLog(name: string, content: string): Promise<ArtifactMetadata> {
    const timestamp = Date.now();
    const filename = `${name}_${timestamp}.log`;
    const filepath = join(this.config.baseDir, "logs", filename);

    await Deno.writeTextFile(filepath, content);

    const metadata: ArtifactMetadata = {
      type: "log",
      path: filepath,
      timestamp,
      testName: name,
    };

    this.registerArtifact(metadata);
    return metadata;
  }

  /**
   * Get the configuration
   */
  getConfig(): ArtifactConfig {
    return { ...this.config };
  }

  /**
   * Check if manager is initialized
   */
  isInitialized(): boolean {
    return this.initialized;
  }
}

/**
 * Create an artifact manager instance
 */
export function createArtifactManager(
  config?: Partial<ArtifactConfig>
): ArtifactManager {
  return new ArtifactManager(config);
}

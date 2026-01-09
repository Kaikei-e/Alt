/**
 * Screenshot capture utilities
 * Provides screenshot capabilities for debugging and artifact collection
 */
import type { Page } from "@astral/astral";
import { ensureDir } from "@std/fs";
import { join } from "@std/path";
import { debug, warn } from "../utils/logger.ts";

/**
 * Screenshot capture options
 */
export interface ScreenshotOptions {
  /** Capture the full scrollable page */
  fullPage: boolean;
  /** Image format */
  format: "png" | "jpeg" | "webp";
  /** Quality for jpeg/webp (0-100) */
  quality?: number;
  /** Clip to specific region */
  clip?: {
    x: number;
    y: number;
    width: number;
    height: number;
  };
}

/**
 * Result of a screenshot capture
 */
export interface ScreenshotResult {
  /** File path where screenshot was saved */
  path: string;
  /** Timestamp when screenshot was taken */
  timestamp: number;
  /** Image dimensions */
  dimensions: { width: number; height: number };
  /** File size in bytes */
  size: number;
}

/**
 * Default screenshot options
 */
export const DEFAULT_SCREENSHOT_OPTIONS: ScreenshotOptions = {
  fullPage: true,
  format: "png",
};

/**
 * Screenshot capture class
 */
export class ScreenshotCapture {
  private outputDir: string;
  private screenshotCount = 0;

  constructor(outputDir: string = "./artifacts/screenshots") {
    this.outputDir = outputDir;
  }

  /**
   * Initialize the output directory
   */
  async initialize(): Promise<void> {
    await ensureDir(this.outputDir);
    debug("Screenshot output directory initialized", { path: this.outputDir });
  }

  /**
   * Capture a screenshot of the current page state
   */
  async capture(
    page: Page,
    name: string,
    options: Partial<ScreenshotOptions> = {}
  ): Promise<ScreenshotResult> {
    const mergedOptions = { ...DEFAULT_SCREENSHOT_OPTIONS, ...options };
    const timestamp = Date.now();
    this.screenshotCount++;

    // Generate filename
    const sanitizedName = name.replace(/[^a-zA-Z0-9-_]/g, "_");
    const filename = `${this.screenshotCount.toString().padStart(3, "0")}_${sanitizedName}_${timestamp}.${mergedOptions.format}`;
    const filepath = join(this.outputDir, filename);

    debug("Capturing screenshot", { name, filepath });

    try {
      // Capture screenshot using Astral
      const screenshotData = await page.screenshot({
        format: mergedOptions.format === "jpeg" ? "jpeg" : "png",
      });

      // Write to file
      await Deno.writeFile(filepath, screenshotData);

      // Get viewport size for dimensions
      const viewport = await page.evaluate(() => ({
        width: globalThis.innerWidth,
        height: document.documentElement.scrollHeight,
      }));

      // Get file size
      const stat = await Deno.stat(filepath);

      const result: ScreenshotResult = {
        path: filepath,
        timestamp,
        dimensions: {
          width: viewport.width,
          height: mergedOptions.fullPage ? viewport.height : viewport.width,
        },
        size: stat.size,
      };

      debug("Screenshot captured successfully", {
        path: filepath,
        size: `${(stat.size / 1024).toFixed(1)}KB`,
      });

      return result;
    } catch (error) {
      warn("Failed to capture screenshot", { name, error: String(error) });
      throw error;
    }
  }

  /**
   * Capture a screenshot when an error occurs
   */
  async captureOnError(
    page: Page,
    error: Error,
    context: string
  ): Promise<ScreenshotResult | null> {
    try {
      const errorName = error.name || "Error";
      const name = `error_${context}_${errorName}`;
      return await this.capture(page, name, { fullPage: true });
    } catch (captureError) {
      warn("Failed to capture error screenshot", {
        context,
        error: String(captureError),
      });
      return null;
    }
  }

  /**
   * Capture a screenshot before and after an operation
   */
  async captureBeforeAfter<T>(
    page: Page,
    operationName: string,
    operation: () => Promise<T>
  ): Promise<{ result: T; before: ScreenshotResult; after: ScreenshotResult }> {
    const before = await this.capture(page, `${operationName}_before`);

    try {
      const result = await operation();
      const after = await this.capture(page, `${operationName}_after`);
      return { result, before, after };
    } catch (error) {
      await this.captureOnError(
        page,
        error instanceof Error ? error : new Error(String(error)),
        operationName
      );
      throw error;
    }
  }

  /**
   * Get the number of screenshots captured
   */
  getScreenshotCount(): number {
    return this.screenshotCount;
  }

  /**
   * Get the output directory path
   */
  getOutputDir(): string {
    return this.outputDir;
  }
}

/**
 * Create a screenshot capture instance
 */
export function createScreenshotCapture(
  outputDir?: string
): ScreenshotCapture {
  return new ScreenshotCapture(outputDir);
}

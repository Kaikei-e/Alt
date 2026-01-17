/**
 * Structured logging for alt-perf
 */
import { bold, cyan, dim, red, yellow } from "./colors.ts";
import { emitOTelLog, initOTelProvider, isOTelEnabled } from "./otel.ts";

// Initialize OTel provider (opt-in via OTEL_ENABLED=true)
let otelShutdown: (() => void) | null = null;

export type LogLevel = "debug" | "info" | "warn" | "error";

interface LoggerConfig {
  level: LogLevel;
  json: boolean;
  verbose: boolean;
}

const LOG_LEVELS: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
};

let config: LoggerConfig = {
  level: "info",
  json: false,
  verbose: false,
};

// OTel-compatible resource attributes
const SERVICE_NAME = Deno.env.get("OTEL_SERVICE_NAME") || "alt-perf";
const SERVICE_VERSION = Deno.env.get("SERVICE_VERSION") || "1.0.0";
const DEPLOYMENT_ENV = Deno.env.get("DEPLOYMENT_ENV") || "development";

// ADR 98/99 business context
interface BusinessContext {
  feedId?: string;
  articleId?: string;
  jobId?: string;
  processingStage?: string;
  aiPipeline?: string;
}

let businessContext: BusinessContext = {};

// Configure logger
export function configureLogger(options: Partial<LoggerConfig>): void {
  config = { ...config, ...options };

  // Initialize OTel provider if not already done
  if (!otelShutdown) {
    otelShutdown = initOTelProvider();
  }
}

// Shutdown OTel provider - call during graceful shutdown
export function shutdownLogger(): void {
  if (otelShutdown) {
    otelShutdown();
    otelShutdown = null;
  }
}

// ADR 98/99 business context setters
export function setFeedId(feedId: string): void {
  businessContext.feedId = feedId;
}

export function setArticleId(articleId: string): void {
  businessContext.articleId = articleId;
}

export function setJobId(jobId: string): void {
  businessContext.jobId = jobId;
}

export function setProcessingStage(stage: string): void {
  businessContext.processingStage = stage;
}

export function setAIPipeline(pipeline: string): void {
  businessContext.aiPipeline = pipeline;
}

export function clearBusinessContext(): void {
  businessContext = {};
}

// Check if level should be logged
function shouldLog(level: LogLevel): boolean {
  return LOG_LEVELS[level] >= LOG_LEVELS[config.level];
}

// Format timestamp
function formatTimestamp(): string {
  return new Date().toISOString();
}

// Log message
function log(
  level: LogLevel,
  message: string,
  data?: Record<string, unknown>
): void {
  if (!shouldLog(level)) return;

  // Emit to OTel if enabled
  if (isOTelEnabled()) {
    const attributes: Record<string, string | number | boolean> = {
      "service.name": SERVICE_NAME,
      "service.version": SERVICE_VERSION,
      "deployment.environment": DEPLOYMENT_ENV,
    };

    // ADR 98/99 business context
    if (businessContext.feedId) attributes["alt.feed.id"] = businessContext.feedId;
    if (businessContext.articleId) attributes["alt.article.id"] = businessContext.articleId;
    if (businessContext.jobId) attributes["alt.job.id"] = businessContext.jobId;
    if (businessContext.processingStage) attributes["alt.processing.stage"] = businessContext.processingStage;
    if (businessContext.aiPipeline) attributes["alt.ai.pipeline"] = businessContext.aiPipeline;

    // Merge data into attributes (only primitive values)
    if (data) {
      for (const [key, value] of Object.entries(data)) {
        if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
          attributes[key] = value;
        }
      }
    }

    emitOTelLog(level, message, attributes);
  }

  if (config.json) {
    const logEntry: Record<string, unknown> = {
      // OTel-compatible fields
      timestamp: formatTimestamp(),
      level,
      msg: message,
      // Resource attributes (OTel semantic conventions)
      "service.name": SERVICE_NAME,
      "service.version": SERVICE_VERSION,
      "deployment.environment": DEPLOYMENT_ENV,
      // ADR 98/99 business context keys
      ...(businessContext.feedId && { "alt.feed.id": businessContext.feedId }),
      ...(businessContext.articleId && { "alt.article.id": businessContext.articleId }),
      ...(businessContext.jobId && { "alt.job.id": businessContext.jobId }),
      ...(businessContext.processingStage && { "alt.processing.stage": businessContext.processingStage }),
      ...(businessContext.aiPipeline && { "alt.ai.pipeline": businessContext.aiPipeline }),
      ...data,
    };
    console.log(JSON.stringify(logEntry));
    return;
  }

  const levelColors: Record<LogLevel, (s: string) => string> = {
    debug: dim,
    info: cyan,
    warn: yellow,
    error: red,
  };

  const colorFn = levelColors[level];
  const prefix = colorFn(`[${level.toUpperCase()}]`);
  const timestamp = dim(formatTimestamp());

  let output = `${timestamp} ${prefix} ${message}`;

  if (data && config.verbose) {
    output += "\n" + dim(JSON.stringify(data, null, 2));
  }

  console.log(output);
}

// Public logging functions
export function debug(message: string, data?: Record<string, unknown>): void {
  log("debug", message, data);
}

export function info(message: string, data?: Record<string, unknown>): void {
  log("info", message, data);
}

export function warn(message: string, data?: Record<string, unknown>): void {
  log("warn", message, data);
}

export function error(message: string, data?: Record<string, unknown>): void {
  log("error", message, data);
}

// Log section header
export function section(title: string): void {
  console.log("");
  console.log(bold(`=== ${title} ===`));
  console.log("");
}

// Log progress
export function progress(current: number, total: number, message: string): void {
  const percent = Math.round((current / total) * 100);
  const bar = createProgressBar(current, total, 20);
  console.log(`${bar} ${percent}% ${dim(message)}`);
}

// Create progress bar
function createProgressBar(current: number, total: number, width: number): string {
  const filled = Math.round((current / total) * width);
  const empty = width - filled;
  return `[${"=".repeat(filled)}${" ".repeat(empty)}]`;
}

// Log timing
export function timing(label: string, durationMs: number): void {
  const formatted = durationMs > 1000
    ? `${(durationMs / 1000).toFixed(2)}s`
    : `${Math.round(durationMs)}ms`;
  info(`${label}: ${formatted}`);
}

// Export current config for testing
export function getConfig(): LoggerConfig {
  return { ...config };
}

/**
 * Structured Logger for alt-frontend (ADR 98/99 compliant)
 *
 * Provides JSON-structured logging with alt.* prefixed business context keys.
 * Works in both server (Node.js) and browser environments.
 */

type LogLevel = "debug" | "info" | "warn" | "error";

interface BusinessContext {
  feedId?: string;
  articleId?: string;
  jobId?: string;
  processingStage?: string;
  aiPipeline?: string;
}

interface LogEntry {
  timestamp: string;
  level: LogLevel;
  message: string;
  "alt.feed.id"?: string;
  "alt.article.id"?: string;
  "alt.job.id"?: string;
  "alt.processing.stage"?: string;
  "alt.ai.pipeline"?: string;
  [key: string]: unknown;
}

const LOG_LEVELS: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
};

function getLogLevel(): LogLevel {
  if (typeof process !== "undefined" && process.env?.LOG_LEVEL) {
    const level = process.env.LOG_LEVEL.toLowerCase() as LogLevel;
    if (level in LOG_LEVELS) {
      return level;
    }
  }
  return "info";
}

function shouldLog(level: LogLevel): boolean {
  const configuredLevel = getLogLevel();
  return LOG_LEVELS[level] >= LOG_LEVELS[configuredLevel];
}

function isServer(): boolean {
  return typeof window === "undefined";
}

class Logger {
  private context: BusinessContext = {};
  private serviceName = "alt-frontend";

  /**
   * Set the feed ID in the logging context
   */
  withFeedId(feedId: string): Logger {
    const logger = new Logger();
    logger.context = { ...this.context, feedId };
    logger.serviceName = this.serviceName;
    return logger;
  }

  /**
   * Set the article ID in the logging context
   */
  withArticleId(articleId: string): Logger {
    const logger = new Logger();
    logger.context = { ...this.context, articleId };
    logger.serviceName = this.serviceName;
    return logger;
  }

  /**
   * Set the job ID in the logging context
   */
  withJobId(jobId: string): Logger {
    const logger = new Logger();
    logger.context = { ...this.context, jobId };
    logger.serviceName = this.serviceName;
    return logger;
  }

  /**
   * Set the processing stage in the logging context
   */
  withProcessingStage(stage: string): Logger {
    const logger = new Logger();
    logger.context = { ...this.context, processingStage: stage };
    logger.serviceName = this.serviceName;
    return logger;
  }

  /**
   * Set the AI pipeline in the logging context
   */
  withAIPipeline(pipeline: string): Logger {
    const logger = new Logger();
    logger.context = { ...this.context, aiPipeline: pipeline };
    logger.serviceName = this.serviceName;
    return logger;
  }

  private formatEntry(
    level: LogLevel,
    message: string,
    data?: Record<string, unknown>,
  ): LogEntry {
    const entry: LogEntry = {
      timestamp: new Date().toISOString(),
      level,
      message,
      service: this.serviceName,
      environment: isServer() ? "server" : "browser",
    };

    // Add business context with alt.* prefix (ADR 98/99)
    if (this.context.feedId) {
      entry["alt.feed.id"] = this.context.feedId;
    }
    if (this.context.articleId) {
      entry["alt.article.id"] = this.context.articleId;
    }
    if (this.context.jobId) {
      entry["alt.job.id"] = this.context.jobId;
    }
    if (this.context.processingStage) {
      entry["alt.processing.stage"] = this.context.processingStage;
    }
    if (this.context.aiPipeline) {
      entry["alt.ai.pipeline"] = this.context.aiPipeline;
    }

    // Add additional data
    if (data) {
      Object.assign(entry, data);
    }

    return entry;
  }

  private output(
    level: LogLevel,
    message: string,
    data?: Record<string, unknown>,
  ): void {
    if (!shouldLog(level)) {
      return;
    }

    const entry = this.formatEntry(level, message, data);

    // On server, output JSON for log aggregation
    if (isServer()) {
      const jsonStr = JSON.stringify(entry);
      switch (level) {
        case "error":
          console.error(jsonStr);
          break;
        case "warn":
          console.warn(jsonStr);
          break;
        default:
          console.log(jsonStr);
      }
    } else {
      // In browser, use structured console methods for better DevTools experience
      switch (level) {
        case "error":
          console.error(`[${level.toUpperCase()}] ${message}`, entry);
          break;
        case "warn":
          console.warn(`[${level.toUpperCase()}] ${message}`, entry);
          break;
        case "info":
          console.info(`[${level.toUpperCase()}] ${message}`, entry);
          break;
        default:
          console.log(`[${level.toUpperCase()}] ${message}`, entry);
      }
    }
  }

  debug(message: string, data?: Record<string, unknown>): void {
    this.output("debug", message, data);
  }

  info(message: string, data?: Record<string, unknown>): void {
    this.output("info", message, data);
  }

  warn(message: string, data?: Record<string, unknown>): void {
    this.output("warn", message, data);
  }

  error(message: string, data?: Record<string, unknown>): void {
    this.output("error", message, data);
  }

  /**
   * Log an error with stack trace
   */
  exception(
    message: string,
    error: Error,
    data?: Record<string, unknown>,
  ): void {
    this.output("error", message, {
      ...data,
      error_name: error.name,
      error_message: error.message,
      error_stack: error.stack,
    });
  }

  /**
   * Log operation duration for performance tracking
   */
  logDuration(
    operation: string,
    durationMs: number,
    data?: Record<string, unknown>,
  ): void {
    this.output("info", `${operation} completed`, {
      ...data,
      operation,
      duration_ms: durationMs,
    });
  }
}

// Export singleton instance
export const logger = new Logger();

// Export class for creating new instances with context
export { Logger };

// Export types for consumers
export type { LogLevel, BusinessContext, LogEntry };

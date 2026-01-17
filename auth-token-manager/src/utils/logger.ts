import { format as formatDate } from "@std/datetime";
import { encodeBase64 } from "@std/encoding/base64";
import {
  ConsoleHandler,
  FileHandler,
  setup as setupLogger,
  LogRecord,
  BaseHandler,
  LoggerConfig,
  LevelName,
  getLogger
} from "@std/log";
import { emitOTelLog, initOTelProvider, isOTelEnabled } from "./otel.ts";

// Basic OAuth token patterns for sanitization
const OAUTH_TOKEN_PATTERNS = [
  // OAuth tokens - Google/Inoreader format
  /ya29\.[A-Za-z0-9\-_]+/g,
  /1\/\/[A-Za-z0-9\-_]+/g,

  // Bearer tokens
  /bearer\s+[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+/gi,

  // Generic API keys (30+ chars)
  /[A-Za-z0-9\-_]{30,}/g,
];

// Sensitive field names to always mask
const SENSITIVE_FIELDS = new Set([
  'access_token', 'refresh_token', 'id_token', 'bearer',
  'password', 'secret', 'key', 'token', 'auth',
  'appid', 'appkey', 'client_secret', 'authorization'
]);

/**
 * Simple data sanitizer for OAuth token logging
 * Focused on CWE-532 prevention
 */
export class DataSanitizer {
  /**
   * Sanitize sensitive data in logs
   */
  static sanitize(data: any): any {
    if (data === null || data === undefined) {
      return data;
    }

    if (typeof data === "string") {
      return this.sanitizeString(data);
    }

    if (Array.isArray(data)) {
      return data.map((item) => this.sanitize(item));
    }

    if (typeof data === "object") {
      return this.sanitizeObject(data);
    }

    return data;
  }

  /**
   * Sanitize strings containing OAuth tokens
   */
  private static sanitizeString(str: string): string {
    let sanitized = str;

    // Apply OAuth token patterns
    for (const pattern of OAUTH_TOKEN_PATTERNS) {
      sanitized = sanitized.replace(pattern, (match) => {
        // Keep first and last few characters for debugging
        if (match.length <= 8) {
          return "[REDACTED]";
        }
        return (
          match.substring(0, 4) +
          "[REDACTED]" +
          match.substring(match.length - 4)
        );
      });
    }

    return sanitized;
  }

  /**
   * Sanitize object properties
   */
  private static sanitizeObject(obj: Record<string, any>): Record<string, any> {
    const sanitized: Record<string, any> = {};

    for (const [key, value] of Object.entries(obj)) {
      const lowerKey = key.toLowerCase();

      if (SENSITIVE_FIELDS.has(lowerKey)) {
        sanitized[key] = "[REDACTED]";
      } else {
        sanitized[key] = this.sanitize(value);
      }
    }

    return sanitized;
  }
}

/**
 * Enhanced log record interface
 */
interface EnhancedLogRecord extends LogRecord {
  component?: string;
  service?: string;
  version?: string;
}

/**
 * ADR 98/99 business context support
 */
interface BusinessContext {
  feedId?: string;
  articleId?: string;
  jobId?: string;
  processingStage?: string;
  aiPipeline?: string;
}

let businessContext: BusinessContext = {};

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

/**
 * JSON formatter for structured logging (OTel-compatible)
 */
class JsonFormatter {
  private serviceName: string;
  private serviceVersion: string;
  private deploymentEnv: string;

  constructor() {
    this.serviceName = Deno.env.get("OTEL_SERVICE_NAME") || "auth-token-manager";
    this.serviceVersion = Deno.env.get("SERVICE_VERSION") || "1.0.0";
    this.deploymentEnv = Deno.env.get("DEPLOYMENT_ENV") || "development";
  }

  format(logRecord: EnhancedLogRecord): string {
    const sanitizedArgs = logRecord.args.map((arg) =>
      DataSanitizer.sanitize(arg)
    );

    const logData: Record<string, unknown> = {
      // OTel-compatible fields
      timestamp: logRecord.datetime.toISOString(),
      level: logRecord.levelName.toLowerCase(),
      msg: DataSanitizer.sanitize(logRecord.msg),
      logger: logRecord.loggerName,
      // Resource attributes (OTel semantic conventions)
      "service.name": this.serviceName,
      "service.version": this.serviceVersion,
      "deployment.environment": this.deploymentEnv,
      // ADR 98/99 business context keys
      ...(businessContext.feedId && { "alt.feed.id": businessContext.feedId }),
      ...(businessContext.articleId && { "alt.article.id": businessContext.articleId }),
      ...(businessContext.jobId && { "alt.job.id": businessContext.jobId }),
      ...(businessContext.processingStage && { "alt.processing.stage": businessContext.processingStage }),
      ...(businessContext.aiPipeline && { "alt.ai.pipeline": businessContext.aiPipeline }),
      // Legacy fields for compatibility
      component: logRecord.component || 'auth-token-manager',
      ...sanitizedArgs.reduce((acc, arg, _index) => {
        if (typeof arg === 'object' && arg !== null) {
          return { ...acc, ...arg };
        }
        return acc;
      }, {})
    };

    return JSON.stringify(logData);
  }
}

/**
 * Custom file handler with JSON formatting
 */
class JsonFileHandler extends FileHandler {
  private jsonFormatter = new JsonFormatter();

  override format(logRecord: LogRecord): string {
    return this.jsonFormatter.format(logRecord as EnhancedLogRecord);
  }
}

// Global OTel shutdown function
let otelShutdown: (() => void) | null = null;

/**
 * Structured logger with OAuth token sanitization and OTel integration
 */
export class StructuredLogger {
  private logger: any;
  private component: string;
  private jsonFormatter: JsonFormatter;

  constructor(component: string) {
    this.component = component;
    this.jsonFormatter = new JsonFormatter();

    // Initialize OTel provider on first logger creation
    if (!otelShutdown) {
      otelShutdown = initOTelProvider();
    }

    // Create log directory if it doesn't exist
    try {
      Deno.mkdirSync("./logs", { recursive: true });
    } catch (error) {
      // Only log in development
      if (Deno.env.get('NODE_ENV') === 'development') {
        console.error("Failed to create log directory:", error);
      }
    }

    // Setup logger configuration - Use only console handler in Kubernetes
    const config: LoggerConfig = {
      handlers: ["console"],
      level: ((Deno.env.get("LOG_LEVEL")?.toUpperCase()) as LevelName) || "INFO",
    };

    // Initialize logger
    setupLogger({
      handlers: {
        console: new ConsoleHandler("DEBUG", {
          formatter: (logRecord) => {
            const enhanced = logRecord as EnhancedLogRecord;
            enhanced.component = this.component;
            return new JsonFormatter().format(enhanced);
          },
        }),
      },
      loggers: {
        [component]: config,
      },
    });

    // Get the configured logger
    this.logger = getLogger(component);
  }

  private emitToOTel(level: string, message: string, args: any[]) {
    if (!isOTelEnabled()) return;

    // Build attributes from args
    const attributes: Record<string, string | number | boolean> = {
      component: this.component,
      "service.name": Deno.env.get("OTEL_SERVICE_NAME") || "auth-token-manager",
    };

    // ADR 98/99 business context
    if (businessContext.feedId) attributes["alt.feed.id"] = businessContext.feedId;
    if (businessContext.articleId) attributes["alt.article.id"] = businessContext.articleId;
    if (businessContext.jobId) attributes["alt.job.id"] = businessContext.jobId;
    if (businessContext.processingStage) attributes["alt.processing.stage"] = businessContext.processingStage;
    if (businessContext.aiPipeline) attributes["alt.ai.pipeline"] = businessContext.aiPipeline;

    // Merge object args into attributes (only primitive values)
    for (const arg of args) {
      if (typeof arg === 'object' && arg !== null) {
        const sanitized = DataSanitizer.sanitize(arg);
        for (const [key, value] of Object.entries(sanitized)) {
          if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
            attributes[key] = value;
          }
        }
      }
    }

    emitOTelLog(level, DataSanitizer.sanitize(message) as string, attributes);
  }

  info(message: string, ...args: any[]) {
    this.logger.info(message, ...args);
    this.emitToOTel("info", message, args);
  }

  warn(message: string, ...args: any[]) {
    this.logger.warn(message, ...args);
    this.emitToOTel("warn", message, args);
  }

  error(message: string, ...args: any[]) {
    this.logger.error(message, ...args);
    this.emitToOTel("error", message, args);
  }

  debug(message: string, ...args: any[]) {
    // Only debug in development
    if (Deno.env.get('NODE_ENV') === 'development') {
      this.logger.debug(message, ...args);
      this.emitToOTel("debug", message, args);
    }
  }
}

/**
 * Shutdown OTel provider - call during graceful shutdown
 */
export function shutdownOTel(): void {
  if (otelShutdown) {
    otelShutdown();
    otelShutdown = null;
  }
}

/**
 * Create component-specific logger
 */
export function createComponentLogger(component: string): StructuredLogger {
  return new StructuredLogger(component);
}

// Export logger singleton
export const logger = createComponentLogger("auth-token-manager");
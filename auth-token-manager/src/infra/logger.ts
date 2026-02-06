import { ConsoleHandler, getLogger, setup as setupLogger } from "@std/log";
import type { LevelName, LoggerConfig, LogRecord } from "@std/log";
import { emitOTelLog, initOTelProvider, isOTelEnabled } from "./otel.ts";

const OAUTH_TOKEN_PATTERNS = [
  /ya29\.[A-Za-z0-9\-_]+/g,
  /1\/\/[A-Za-z0-9\-_]+/g,
  /bearer\s+[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+/gi,
  /[A-Za-z0-9\-_]{30,}/g,
];

const SENSITIVE_FIELDS = new Set([
  "access_token",
  "refresh_token",
  "id_token",
  "bearer",
  "password",
  "secret",
  "key",
  "token",
  "auth",
  "appid",
  "appkey",
  "client_secret",
  "authorization",
]);

export class DataSanitizer {
  // deno-lint-ignore no-explicit-any
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

  private static sanitizeString(str: string): string {
    let sanitized = str;
    for (const pattern of OAUTH_TOKEN_PATTERNS) {
      sanitized = sanitized.replace(pattern, (match) => {
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

  // deno-lint-ignore no-explicit-any
  private static sanitizeObject(obj: Record<string, any>): Record<string, any> {
    // deno-lint-ignore no-explicit-any
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

interface EnhancedLogRecord extends LogRecord {
  component?: string;
}

interface BusinessContext {
  feedId?: string;
  articleId?: string;
  jobId?: string;
  processingStage?: string;
  aiPipeline?: string;
}

let businessContext: BusinessContext = {};

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

class JsonFormatter {
  private serviceName: string;
  private serviceVersion: string;
  private deploymentEnv: string;

  constructor() {
    this.serviceName = Deno.env.get("OTEL_SERVICE_NAME") ||
      "auth-token-manager";
    this.serviceVersion = Deno.env.get("SERVICE_VERSION") || "1.0.0";
    this.deploymentEnv = Deno.env.get("DEPLOYMENT_ENV") || "development";
  }

  format(logRecord: EnhancedLogRecord): string {
    const sanitizedArgs = logRecord.args.map((arg) =>
      DataSanitizer.sanitize(arg)
    );

    const logData: Record<string, unknown> = {
      timestamp: logRecord.datetime.toISOString(),
      level: logRecord.levelName.toLowerCase(),
      msg: DataSanitizer.sanitize(logRecord.msg),
      logger: logRecord.loggerName,
      "service.name": this.serviceName,
      "service.version": this.serviceVersion,
      "deployment.environment": this.deploymentEnv,
      ...(businessContext.feedId && { "alt.feed.id": businessContext.feedId }),
      ...(businessContext.articleId && {
        "alt.article.id": businessContext.articleId,
      }),
      ...(businessContext.jobId && { "alt.job.id": businessContext.jobId }),
      ...(businessContext.processingStage && {
        "alt.processing.stage": businessContext.processingStage,
      }),
      ...(businessContext.aiPipeline && {
        "alt.ai.pipeline": businessContext.aiPipeline,
      }),
      component: logRecord.component || "auth-token-manager",
      ...sanitizedArgs.reduce((acc, arg) => {
        if (typeof arg === "object" && arg !== null) {
          return { ...acc, ...arg };
        }
        return acc;
      }, {}),
    };

    return JSON.stringify(logData);
  }
}

let otelShutdown: (() => void) | null = null;

export class StructuredLogger {
  // deno-lint-ignore no-explicit-any
  private logger: any;
  private component: string;

  constructor(component: string) {
    this.component = component;

    if (!otelShutdown) {
      otelShutdown = initOTelProvider();
    }

    try {
      Deno.mkdirSync("./logs", { recursive: true });
    } catch {
      // Ignore - logs directory creation is best-effort
    }

    const logConfig: LoggerConfig = {
      handlers: ["console"],
      level: ((Deno.env.get("LOG_LEVEL")?.toUpperCase()) as LevelName) ||
        "INFO",
    };

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
        [component]: logConfig,
      },
    });

    this.logger = getLogger(component);
  }

  // deno-lint-ignore no-explicit-any
  private emitToOTel(level: string, message: string, args: any[]) {
    if (!isOTelEnabled()) return;

    const attributes: Record<string, string | number | boolean> = {
      component: this.component,
      "service.name": Deno.env.get("OTEL_SERVICE_NAME") ||
        "auth-token-manager",
    };

    if (businessContext.feedId) {
      attributes["alt.feed.id"] = businessContext.feedId;
    }
    if (businessContext.articleId) {
      attributes["alt.article.id"] = businessContext.articleId;
    }
    if (businessContext.jobId) {
      attributes["alt.job.id"] = businessContext.jobId;
    }
    if (businessContext.processingStage) {
      attributes["alt.processing.stage"] = businessContext.processingStage;
    }
    if (businessContext.aiPipeline) {
      attributes["alt.ai.pipeline"] = businessContext.aiPipeline;
    }

    for (const arg of args) {
      if (typeof arg === "object" && arg !== null) {
        const sanitized = DataSanitizer.sanitize(arg);
        for (const [key, value] of Object.entries(sanitized)) {
          if (
            typeof value === "string" || typeof value === "number" ||
            typeof value === "boolean"
          ) {
            attributes[key] = value;
          }
        }
      }
    }

    emitOTelLog(level, DataSanitizer.sanitize(message) as string, attributes);
  }

  // deno-lint-ignore no-explicit-any
  info(message: string, ...args: any[]) {
    this.logger.info(message, ...args);
    this.emitToOTel("info", message, args);
  }

  // deno-lint-ignore no-explicit-any
  warn(message: string, ...args: any[]) {
    this.logger.warn(message, ...args);
    this.emitToOTel("warn", message, args);
  }

  // deno-lint-ignore no-explicit-any
  error(message: string, ...args: any[]) {
    this.logger.error(message, ...args);
    this.emitToOTel("error", message, args);
  }

  // deno-lint-ignore no-explicit-any
  debug(message: string, ...args: any[]) {
    if (Deno.env.get("NODE_ENV") === "development") {
      this.logger.debug(message, ...args);
      this.emitToOTel("debug", message, args);
    }
  }
}

export function shutdownOTel(): void {
  if (otelShutdown) {
    otelShutdown();
    otelShutdown = null;
  }
}

export function createComponentLogger(component: string): StructuredLogger {
  return new StructuredLogger(component);
}

export const logger = createComponentLogger("auth-token-manager");

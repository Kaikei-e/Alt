import { ConsoleHandler, getLogger, setup as setupLogger } from "@std/log";
import type { LevelName, LoggerConfig, LogRecord } from "@std/log";
import { emitOTelLog, initOTelProvider, isOTelEnabled } from "./otel.ts";

const OAUTH_TOKEN_PATTERNS = [
  /ya29\.[A-Za-z0-9\-_]+/g,
  /1\/\/[A-Za-z0-9\-_]+/g,
  /bearer\s+[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+/gi,
  /[A-Za-z0-9\-_]{30,}/g,
];

const SENSITIVE_FIELDS = [
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
  "clientsecret",
  "authorization",
];

/** Normalize field names for matching: strip separators, lowercase. */
function normalizeFieldName(key: string): string {
  return key.toLowerCase().replace(/[-_\s]/g, "");
}

function isSensitiveField(key: string): boolean {
  const normalized = normalizeFieldName(key);
  return SENSITIVE_FIELDS.some((field) =>
    normalized.includes(normalizeFieldName(field))
  );
}

export class DataSanitizer {
  static sanitize(data: unknown): unknown {
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
      return this.sanitizeObject(data as Record<string, unknown>);
    }
    return data;
  }

  private static sanitizeString(str: string): string {
    let sanitized = str;
    for (const pattern of OAUTH_TOKEN_PATTERNS) {
      sanitized = sanitized.replace(pattern, "[REDACTED]");
    }
    return sanitized;
  }

  private static sanitizeObject(
    obj: Record<string, unknown>,
  ): Record<string, unknown> {
    const sanitized: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(obj)) {
      if (isSensitiveField(key)) {
        sanitized[key] = "[REDACTED]";
      } else {
        sanitized[key] = this.sanitize(value);
      }
    }
    return sanitized;
  }
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

  format(logRecord: LogRecord): string {
    // The active component is passed as a leading arg on every call (see
    // StructuredLogger methods below) rather than mutated onto the shared
    // LogRecord, since the handler/formatter is registered once for all
    // components.
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
      component: "auth-token-manager",
      ...sanitizedArgs.reduce((acc: Record<string, unknown>, arg) => {
        if (typeof arg === "object" && arg !== null) {
          return { ...acc, ...(arg as Record<string, unknown>) };
        }
        return acc;
      }, {}),
    };

    return JSON.stringify(logData);
  }
}

let otelShutdown: (() => Promise<void>) | null = null;

const LOGGER_NAME = "app";
let loggerSetupDone = false;

function ensureLoggerSetup(): void {
  if (loggerSetupDone) return;
  loggerSetupDone = true;

  const logConfig: LoggerConfig = {
    handlers: ["console"],
    level: ((Deno.env.get("LOG_LEVEL")?.toUpperCase()) as LevelName) ||
      "INFO",
  };

  setupLogger({
    handlers: {
      console: new ConsoleHandler("DEBUG", {
        formatter: (logRecord) => new JsonFormatter().format(logRecord),
      }),
    },
    loggers: {
      [LOGGER_NAME]: logConfig,
    },
  });
}

export class StructuredLogger {
  private logger: ReturnType<typeof getLogger>;
  private component: string;

  constructor(component: string) {
    this.component = component;

    if (!otelShutdown) {
      otelShutdown = initOTelProvider();
    }

    ensureLoggerSetup();
    this.logger = getLogger(LOGGER_NAME);
  }

  private emitToOTel(level: string, message: string, args: unknown[]) {
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
        if (typeof sanitized === "object" && sanitized !== null) {
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
    }

    const sanitizedMessage = DataSanitizer.sanitize(message);
    emitOTelLog(
      level,
      typeof sanitizedMessage === "string" ? sanitizedMessage : message,
      attributes,
    );
  }

  info(message: string, ...args: unknown[]) {
    this.logger.info(message, { component: this.component }, ...args);
    this.emitToOTel("info", message, args);
  }

  warn(message: string, ...args: unknown[]) {
    this.logger.warn(message, { component: this.component }, ...args);
    this.emitToOTel("warn", message, args);
  }

  error(message: string, ...args: unknown[]) {
    this.logger.error(message, { component: this.component }, ...args);
    this.emitToOTel("error", message, args);
  }

  debug(message: string, ...args: unknown[]) {
    // Gated by the "app" logger's configured level (LOG_LEVEL), not NODE_ENV,
    // so LOG_LEVEL=DEBUG works in production without a development flag.
    this.logger.debug(message, { component: this.component }, ...args);
    this.emitToOTel("debug", message, args);
  }
}

export async function shutdownOTel(): Promise<void> {
  if (otelShutdown) {
    await otelShutdown();
    otelShutdown = null;
  }
}

export function createComponentLogger(component: string): StructuredLogger {
  return new StructuredLogger(component);
}

export const logger = createComponentLogger("auth-token-manager");

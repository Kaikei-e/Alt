/**
 * OpenTelemetry provider for alt-perf CLI tool.
 * Handles optional OTLP log export for observability.
 */

import { logs, SeverityNumber } from "@opentelemetry/api-logs";
import { LoggerProvider, BatchLogRecordProcessor } from "@opentelemetry/sdk-logs";
import { OTLPLogExporter } from "@opentelemetry/exporter-logs-otlp-http";
import { Resource } from "@opentelemetry/resources";
import {
  SEMRESATTRS_SERVICE_NAME,
  SEMRESATTRS_SERVICE_VERSION,
  SEMRESATTRS_DEPLOYMENT_ENVIRONMENT,
} from "@opentelemetry/semantic-conventions";

/**
 * OTel configuration from environment variables
 */
export interface OTelConfig {
  serviceName: string;
  serviceVersion: string;
  environment: string;
  otlpEndpoint: string;
  enabled: boolean;
}

/**
 * Get OTel configuration from environment
 */
export function getOTelConfig(): OTelConfig {
  return {
    serviceName: Deno.env.get("OTEL_SERVICE_NAME") || "alt-perf",
    serviceVersion: Deno.env.get("SERVICE_VERSION") || "1.0.0",
    environment: Deno.env.get("DEPLOYMENT_ENV") || "development",
    otlpEndpoint: Deno.env.get("OTEL_EXPORTER_OTLP_ENDPOINT") || "http://localhost:4318",
    // Default to false for CLI tool (opt-in)
    enabled: (Deno.env.get("OTEL_ENABLED") || "false").toLowerCase() === "true",
  };
}

let loggerProvider: LoggerProvider | null = null;
let otelLogger: ReturnType<typeof logs.getLogger> | null = null;

/**
 * Initialize OpenTelemetry logger provider
 */
export function initOTelProvider(config?: OTelConfig): () => void {
  const cfg = config || getOTelConfig();

  if (!cfg.enabled) {
    return () => {};
  }

  // Create resource with service information
  const resource = new Resource({
    [SEMRESATTRS_SERVICE_NAME]: cfg.serviceName,
    [SEMRESATTRS_SERVICE_VERSION]: cfg.serviceVersion,
    [SEMRESATTRS_DEPLOYMENT_ENVIRONMENT]: cfg.environment,
  });

  // Create OTLP log exporter
  const logExporter = new OTLPLogExporter({
    url: `${cfg.otlpEndpoint}/v1/logs`,
  });

  // Create logger provider
  loggerProvider = new LoggerProvider({
    resource,
  });

  loggerProvider.addLogRecordProcessor(new BatchLogRecordProcessor(logExporter));

  // Set as global provider
  logs.setGlobalLoggerProvider(loggerProvider);

  // Get logger instance
  otelLogger = logs.getLogger("alt-perf");

  return () => {
    if (loggerProvider) {
      loggerProvider.shutdown();
      loggerProvider = null;
      otelLogger = null;
    }
  };
}

/**
 * Map log level string to OTel severity number
 */
function levelToSeverity(level: string): SeverityNumber {
  switch (level.toLowerCase()) {
    case "error":
      return SeverityNumber.ERROR;
    case "warn":
    case "warning":
      return SeverityNumber.WARN;
    case "info":
      return SeverityNumber.INFO;
    case "debug":
      return SeverityNumber.DEBUG;
    default:
      return SeverityNumber.INFO;
  }
}

/**
 * Emit a log record to OTel
 */
export function emitOTelLog(
  level: string,
  message: string,
  attributes: Record<string, string | number | boolean> = {}
): void {
  if (!otelLogger) {
    return;
  }

  // Filter out undefined/null values and ensure valid types
  const cleanAttributes: Record<string, string | number | boolean> = {};
  for (const [key, value] of Object.entries(attributes)) {
    if (value !== undefined && value !== null) {
      if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
        cleanAttributes[key] = value;
      } else {
        cleanAttributes[key] = String(value);
      }
    }
  }

  otelLogger.emit({
    severityNumber: levelToSeverity(level),
    severityText: level.toUpperCase(),
    body: message,
    attributes: cleanAttributes,
  });
}

/**
 * Check if OTel is enabled and initialized
 */
export function isOTelEnabled(): boolean {
  return otelLogger !== null;
}

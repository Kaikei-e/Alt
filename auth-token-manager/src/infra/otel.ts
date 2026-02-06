/**
 * OpenTelemetry provider for auth-token-manager service.
 */

import { logs, SeverityNumber } from "@opentelemetry/api-logs";
import {
  BatchLogRecordProcessor,
  LoggerProvider,
} from "@opentelemetry/sdk-logs";
import { OTLPLogExporter } from "@opentelemetry/exporter-logs-otlp-http";
import { resourceFromAttributes } from "@opentelemetry/resources";
import {
  ATTR_SERVICE_NAME,
  ATTR_SERVICE_VERSION,
} from "@opentelemetry/semantic-conventions";

const ATTR_DEPLOYMENT_ENVIRONMENT = "deployment.environment";

export interface OTelConfig {
  serviceName: string;
  serviceVersion: string;
  environment: string;
  otlpEndpoint: string;
  enabled: boolean;
}

export function getOTelConfig(): OTelConfig {
  return {
    serviceName: Deno.env.get("OTEL_SERVICE_NAME") || "auth-token-manager",
    serviceVersion: Deno.env.get("SERVICE_VERSION") || "1.0.0",
    environment: Deno.env.get("DEPLOYMENT_ENV") || "development",
    otlpEndpoint: Deno.env.get("OTEL_EXPORTER_OTLP_ENDPOINT") ||
      "http://localhost:4318",
    enabled: (Deno.env.get("OTEL_ENABLED") || "true").toLowerCase() === "true",
  };
}

let loggerProvider: LoggerProvider | null = null;
let otelLogger: ReturnType<typeof logs.getLogger> | null = null;

export function initOTelProvider(config?: OTelConfig): () => void {
  const cfg = config || getOTelConfig();

  if (!cfg.enabled) {
    return () => {};
  }

  const resource = resourceFromAttributes({
    [ATTR_SERVICE_NAME]: cfg.serviceName,
    [ATTR_SERVICE_VERSION]: cfg.serviceVersion,
    [ATTR_DEPLOYMENT_ENVIRONMENT]: cfg.environment,
  });

  const logExporter = new OTLPLogExporter({
    url: `${cfg.otlpEndpoint}/v1/logs`,
  });

  loggerProvider = new LoggerProvider({
    resource,
    processors: [new BatchLogRecordProcessor(logExporter)],
  });

  logs.setGlobalLoggerProvider(loggerProvider);
  otelLogger = logs.getLogger("auth-token-manager");

  return () => {
    if (loggerProvider) {
      loggerProvider.shutdown();
      loggerProvider = null;
      otelLogger = null;
    }
  };
}

function levelToSeverity(level: string): SeverityNumber {
  switch (level.toLowerCase()) {
    case "error":
    case "critical":
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

export function emitOTelLog(
  level: string,
  message: string,
  attributes: Record<string, string | number | boolean> = {},
): void {
  if (!otelLogger) {
    return;
  }

  const cleanAttributes: Record<string, string | number | boolean> = {};
  for (const [key, value] of Object.entries(attributes)) {
    if (value !== undefined && value !== null) {
      if (
        typeof value === "string" || typeof value === "number" ||
        typeof value === "boolean"
      ) {
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

export function isOTelEnabled(): boolean {
  return otelLogger !== null;
}

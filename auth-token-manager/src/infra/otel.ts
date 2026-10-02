/**
 * OpenTelemetry provider for auth-token-manager service.
 */

import { trace } from "@opentelemetry/api";
import { logs, SeverityNumber } from "@opentelemetry/api-logs";
import {
  BatchLogRecordProcessor,
  LoggerProvider,
} from "@opentelemetry/sdk-logs";
import {
  BasicTracerProvider,
  BatchSpanProcessor,
} from "@opentelemetry/sdk-trace-base";
import { OTLPLogExporter } from "@opentelemetry/exporter-logs-otlp-proto";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-proto";
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

export function loadRaskIngestToken(): string {
  const tokenFile = Deno.env.get("RASK_INGEST_TOKEN_FILE");
  if (!tokenFile) {
    throw new Error("RASK_INGEST_TOKEN_FILE must be set to export telemetry");
  }

  let content: string;
  try {
    content = Deno.readTextFileSync(tokenFile);
  } catch (err) {
    throw new Error(`Failed to read RASK_INGEST_TOKEN_FILE: ${err instanceof Error ? err.message : String(err)}`);
  }

  // Strip exactly one trailing CRLF or LF; do NOT trim whitespace broadly.
  // content.trim() would silently accept leading/trailing spaces and Unicode
  // whitespace – those are NOT valid in a Bearer token file.
  const token = content.replace(/\r?\n$/, "");
  if (!token) {
    throw new Error("RASK_INGEST_TOKEN_FILE is empty");
  }

  // RFC 6750 §2.1 token68: [A-Za-z0-9._~+/\-]+ followed by optional = padding.
  // '-' must be present: URL-safe base64url and many token generators include it.
  if (!/^[A-Za-z0-9._~+/\-]+=*$/.test(token)) {
    throw new Error("RASK_INGEST_TOKEN_FILE contains invalid characters");
  }

  return token;
}

export function getOTelConfig(): OTelConfig {
  const env = Deno.env.get("DEPLOYMENT_ENV") || "development";
  return {
    serviceName: Deno.env.get("OTEL_SERVICE_NAME") || "auth-token-manager",
    serviceVersion: Deno.env.get("SERVICE_VERSION") || "1.0.0",
    environment: env,
    otlpEndpoint: Deno.env.get("OTEL_EXPORTER_OTLP_ENDPOINT") ||
      "http://localhost:4318",
    enabled: Deno.env.get("OTEL_ENABLED")?.toLowerCase() === "true",
  };
}

let loggerProvider: LoggerProvider | null = null;
let tracerProvider: BasicTracerProvider | null = null;
let otelLogger: ReturnType<typeof logs.getLogger> | null = null;

export function initOTelProvider(config?: OTelConfig): () => Promise<void> {
  const cfg = config || getOTelConfig();

  if (!cfg.enabled) {
    return () => Promise.resolve();
  }

  const token = loadRaskIngestToken();

  const resource = resourceFromAttributes({
    [ATTR_SERVICE_NAME]: cfg.serviceName,
    [ATTR_SERVICE_VERSION]: cfg.serviceVersion,
    [ATTR_DEPLOYMENT_ENVIRONMENT]: cfg.environment,
  });

  const logExporter = new OTLPLogExporter({
    url: `${cfg.otlpEndpoint}/v1/logs`,
    headers: {
      "Authorization": `Bearer ${token}`
    }
  });

  const traceExporter = new OTLPTraceExporter({
    url: `${cfg.otlpEndpoint}/v1/traces`,
    headers: {
      "Authorization": `Bearer ${token}`
    }
  });

  loggerProvider = new LoggerProvider({
    resource,
    processors: [new BatchLogRecordProcessor({ exporter: logExporter })],
  });

  tracerProvider = new BasicTracerProvider({
    resource,
    spanProcessors: [new BatchSpanProcessor(traceExporter)],
  });

  logs.setGlobalLoggerProvider(loggerProvider);
  trace.setGlobalTracerProvider(tracerProvider);
  otelLogger = logs.getLogger("auth-token-manager");

  return async () => {
    const lProvider = loggerProvider;
    const tProvider = tracerProvider;
    loggerProvider = null;
    tracerProvider = null;
    otelLogger = null;
    const promises: Promise<unknown>[] = [];
    if (lProvider) {
      promises.push(lProvider.shutdown());
    }
    if (tProvider) {
      promises.push(tProvider.shutdown());
    }
    await Promise.all(promises);
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

/**
 * Structured logging for alt-perf
 */
import { bold, cyan, dim, red, yellow } from "./colors.ts";

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

// Configure logger
export function configureLogger(options: Partial<LoggerConfig>): void {
  config = { ...config, ...options };
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

  if (config.json) {
    const logEntry = {
      timestamp: formatTimestamp(),
      level,
      message,
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

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
 * JSON formatter for structured logging
 */
class JsonFormatter {
  format(logRecord: EnhancedLogRecord): string {
    const sanitizedArgs = logRecord.args.map((arg) => 
      DataSanitizer.sanitize(arg)
    );

    const logData = {
      timestamp: logRecord.datetime.toISOString(),
      level: logRecord.levelName,
      message: DataSanitizer.sanitize(logRecord.msg),
      logger: logRecord.loggerName,
      component: logRecord.component || 'auth-token-manager',
      service: 'auth-token-manager',
      version: '1.0.0',
      ...sanitizedArgs.reduce((acc, arg, index) => {
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

/**
 * Structured logger with OAuth token sanitization
 */
export class StructuredLogger {
  private logger: any;
  private component: string;

  constructor(component: string) {
    this.component = component;
    
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
      level: (Deno.env.get("LOG_LEVEL") as LevelName) || "INFO",
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

  info(message: string, ...args: any[]) {
    this.logger.info(message, ...args);
  }

  warn(message: string, ...args: any[]) {
    this.logger.warn(message, ...args);
  }

  error(message: string, ...args: any[]) {
    this.logger.error(message, ...args);
  }

  debug(message: string, ...args: any[]) {
    // Only debug in development
    if (Deno.env.get('NODE_ENV') === 'development') {
      this.logger.debug(message, ...args);
    }
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
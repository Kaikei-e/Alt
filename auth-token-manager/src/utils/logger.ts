/**
 * Structured JSON Logging System
 * 
 * This module provides comprehensive structured logging for the OAuth token manager
 * with support for different log levels, correlation IDs, and JSON formatting
 * suitable for log aggregation systems.
 */

import { LogLevelName, LevelName, LogRecord, ConsoleHandler, FileHandler, setup } from '@std/log';
import { format } from '@std/datetime';
import type { AppConfig } from '../auth/types.ts';

/**
 * Enhanced log record with additional metadata
 */
interface EnhancedLogRecord extends LogRecord {
  extra?: Record<string, unknown>;
  correlation_id?: string;
  session_id?: string;
  request_id?: string;
  user_id?: string;
  component?: string;
  operation?: string;
}

/**
 * Custom JSON formatter for structured logging
 */
class JsonFormatter {
  format(logRecord: EnhancedLogRecord): string {
    const timestamp = format(logRecord.datetime, 'yyyy-MM-ddTHH:mm:ss.SSSZ');
    
    const logEntry = {
      timestamp,
      level: logRecord.levelName,
      message: logRecord.msg,
      logger: logRecord.loggerName,
      ...(logRecord.extra && Object.keys(logRecord.extra).length > 0 && { ...logRecord.extra }),
      ...(logRecord.correlation_id && { correlation_id: logRecord.correlation_id }),
      ...(logRecord.session_id && { session_id: logRecord.session_id }),
      ...(logRecord.request_id && { request_id: logRecord.request_id }),
      ...(logRecord.user_id && { user_id: logRecord.user_id }),
      ...(logRecord.component && { component: logRecord.component }),
      ...(logRecord.operation && { operation: logRecord.operation }),
      service: 'auth-token-manager',
      version: Deno.env.get('APP_VERSION') || '1.0.0',
      environment: Deno.env.get('ENVIRONMENT') || 'development'
    };

    return JSON.stringify(logEntry);
  }
}

/**
 * Custom console handler with JSON formatting
 */
class JsonConsoleHandler extends ConsoleHandler {
  private jsonFormatter = new JsonFormatter();

  format(logRecord: LogRecord): string {
    return this.jsonFormatter.format(logRecord as EnhancedLogRecord);
  }
}

/**
 * Custom file handler with JSON formatting
 */
class JsonFileHandler extends FileHandler {
  private jsonFormatter = new JsonFormatter();

  format(logRecord: LogRecord): string {
    return this.jsonFormatter.format(logRecord as EnhancedLogRecord);
  }
}

/**
 * Logger context for maintaining correlation IDs and metadata
 */
class LoggerContext {
  private static instance: LoggerContext;
  private context: Map<string, Record<string, unknown>> = new Map();
  
  static getInstance(): LoggerContext {
    if (!LoggerContext.instance) {
      LoggerContext.instance = new LoggerContext();
    }
    return LoggerContext.instance;
  }

  setContext(key: string, value: Record<string, unknown>): void {
    this.context.set(key, value);
  }

  getContext(key: string): Record<string, unknown> | undefined {
    return this.context.get(key);
  }

  clearContext(key: string): void {
    this.context.delete(key);
  }

  getAllContext(): Record<string, unknown> {
    const allContext: Record<string, unknown> = {};
    for (const [key, value] of this.context.entries()) {
      Object.assign(allContext, value);
    }
    return allContext;
  }

  generateCorrelationId(): string {
    const array = new Uint8Array(16);
    crypto.getRandomValues(array);
    return Array.from(array, byte => byte.toString(16).padStart(2, '0')).join('');
  }
}

/**
 * Enhanced logger wrapper with structured logging capabilities
 */
class StructuredLogger {
  private loggerName: string;
  private context: LoggerContext;

  constructor(loggerName: string = 'auth-token-manager') {
    this.loggerName = loggerName;
    this.context = LoggerContext.getInstance();
  }

  /**
   * Log debug message with optional metadata
   */
  debug(message: string, extra?: Record<string, unknown>): void {
    this.log('DEBUG', message, extra);
  }

  /**
   * Log info message with optional metadata
   */
  info(message: string, extra?: Record<string, unknown>): void {
    this.log('INFO', message, extra);
  }

  /**
   * Log warning message with optional metadata
   */
  warn(message: string, extra?: Record<string, unknown>): void {
    this.log('WARN', message, extra);
  }

  /**
   * Log error message with optional metadata
   */
  error(message: string, extra?: Record<string, unknown>): void {
    this.log('ERROR', message, extra);
  }

  /**
   * Log critical error message with optional metadata
   */
  critical(message: string, extra?: Record<string, unknown>): void {
    this.log('CRITICAL', message, extra);
  }

  /**
   * Log with operation context
   */
  logOperation(
    level: LevelName,
    operation: string,
    message: string,
    extra?: Record<string, unknown>
  ): void {
    this.log(level, message, { ...extra, operation });
  }

  /**
   * Log with component context
   */
  logComponent(
    level: LevelName,
    component: string,
    message: string,
    extra?: Record<string, unknown>
  ): void {
    this.log(level, message, { ...extra, component });
  }

  /**
   * Start timing an operation
   */
  startTiming(operationName: string): () => void {
    const startTime = performance.now();
    const correlationId = this.context.generateCorrelationId();
    
    this.info(`Starting ${operationName}`, {
      operation: operationName,
      correlation_id: correlationId,
      timing: 'start'
    });

    return () => {
      const duration = performance.now() - startTime;
      this.info(`Completed ${operationName}`, {
        operation: operationName,
        correlation_id: correlationId,
        timing: 'end',
        duration_ms: Math.round(duration)
      });
    };
  }

  /**
   * Log performance metrics
   */
  logMetrics(metrics: Record<string, number | string>, component?: string): void {
    this.info('Performance metrics', {
      ...metrics,
      ...(component && { component }),
      metric_type: 'performance'
    });
  }

  /**
   * Log security event
   */
  logSecurity(event: string, details: Record<string, unknown>): void {
    this.warn(`Security event: ${event}`, {
      ...details,
      event_type: 'security',
      severity: 'high'
    });
  }

  /**
   * Log audit event
   */
  logAudit(action: string, details: Record<string, unknown>): void {
    this.info(`Audit: ${action}`, {
      ...details,
      event_type: 'audit',
      timestamp: new Date().toISOString()
    });
  }

  /**
   * Set correlation ID for subsequent logs
   */
  setCorrelationId(correlationId: string): void {
    this.context.setContext('correlation', { correlation_id: correlationId });
  }

  /**
   * Set session ID for subsequent logs
   */
  setSessionId(sessionId: string): void {
    this.context.setContext('session', { session_id: sessionId });
  }

  /**
   * Set request ID for subsequent logs
   */
  setRequestId(requestId: string): void {
    this.context.setContext('request', { request_id: requestId });
  }

  /**
   * Set user ID for subsequent logs
   */
  setUserId(userId: string): void {
    this.context.setContext('user', { user_id: userId });
  }

  /**
   * Clear all context
   */
  clearContext(): void {
    this.context = LoggerContext.getInstance();
  }

  /**
   * Create child logger with additional context
   */
  child(additionalContext: Record<string, unknown>): StructuredLogger {
    const childLogger = new StructuredLogger(this.loggerName);
    childLogger.context.setContext('parent', this.context.getAllContext());
    childLogger.context.setContext('child', additionalContext);
    return childLogger;
  }

  /**
   * Internal log method with context injection
   */
  private log(level: LevelName, message: string, extra?: Record<string, unknown>): void {
    const logger = globalThis.console;
    const contextData = this.context.getAllContext();
    
    const enhancedRecord: EnhancedLogRecord = {
      msg: message,
      args: [],
      datetime: new Date(),
      level: this.mapLogLevel(level),
      levelName: level,
      loggerName: this.loggerName,
      extra: { ...contextData, ...extra }
    };

    const formattedMessage = new JsonFormatter().format(enhancedRecord);

    switch (level) {
      case 'DEBUG':
        logger.debug(formattedMessage);
        break;
      case 'INFO':
        logger.info(formattedMessage);
        break;
      case 'WARN':
        logger.warn(formattedMessage);
        break;
      case 'ERROR':
      case 'CRITICAL':
        logger.error(formattedMessage);
        break;
      default:
        logger.log(formattedMessage);
    }
  }

  /**
   * Map log level names to numeric levels
   */
  private mapLogLevel(levelName: LevelName): number {
    const levels: Record<LevelName, number> = {
      'NOTSET': 0,
      'DEBUG': 10,
      'INFO': 20,
      'WARN': 30,
      'ERROR': 40,
      'CRITICAL': 50
    };
    return levels[levelName] || 20;
  }
}

/**
 * Initialize logging system with configuration
 */
export async function initializeLogging(config?: AppConfig): Promise<void> {
  const logLevel = (config?.log_level || Deno.env.get('LOG_LEVEL') || 'INFO').toUpperCase() as LogLevelName;
  const environment = config?.environment || Deno.env.get('ENVIRONMENT') || 'development';

  // Console handler for all environments
  const handlers: Record<string, any> = {
    console: new JsonConsoleHandler(logLevel, {
      formatter: new JsonFormatter()
    })
  };

  // File handler for production environment
  if (environment === 'production') {
    const logDir = Deno.env.get('LOG_DIR') || './logs';
    
    try {
      await Deno.mkdir(logDir, { recursive: true });
      
      handlers.file = new JsonFileHandler(logLevel, {
        filename: `${logDir}/auth-token-manager.log`,
        formatter: new JsonFormatter()
      });
    } catch (error) {
      console.error('Failed to create log directory:', error);
    }
  }

  // Setup logging configuration
  await setup({
    handlers,
    loggers: {
      'auth-token-manager': {
        level: logLevel,
        handlers: Object.keys(handlers)
      },
      'browser': {
        level: logLevel,
        handlers: Object.keys(handlers)
      },
      'oauth': {
        level: logLevel,
        handlers: Object.keys(handlers)
      },
      'k8s': {
        level: logLevel,
        handlers: Object.keys(handlers)
      }
    }
  });

  // Log initialization
  const logger = new StructuredLogger();
  logger.info('Logging system initialized', {
    log_level: logLevel,
    environment,
    handlers: Object.keys(handlers),
    component: 'logger'
  });
}

/**
 * Performance monitoring decorator
 */
export function logPerformance(operation: string) {
  return function (target: any, propertyKey: string, descriptor: PropertyDescriptor) {
    const originalMethod = descriptor.value;

    descriptor.value = async function (...args: any[]) {
      const logger = new StructuredLogger();
      const endTiming = logger.startTiming(`${target.constructor.name}.${propertyKey}`);
      
      try {
        const result = await originalMethod.apply(this, args);
        endTiming();
        return result;
      } catch (error) {
        endTiming();
        logger.error(`Operation ${operation} failed`, {
          error: error.message,
          operation,
          method: `${target.constructor.name}.${propertyKey}`
        });
        throw error;
      }
    };

    return descriptor;
  };
}

/**
 * Error logging decorator
 */
export function logErrors(component: string) {
  return function (target: any, propertyKey: string, descriptor: PropertyDescriptor) {
    const originalMethod = descriptor.value;

    descriptor.value = async function (...args: any[]) {
      const logger = new StructuredLogger();
      
      try {
        return await originalMethod.apply(this, args);
      } catch (error) {
        logger.error(`Error in ${component}`, {
          error: error.message,
          stack: error.stack,
          component,
          method: `${target.constructor.name}.${propertyKey}`,
          args: args.length
        });
        throw error;
      }
    };

    return descriptor;
  };
}

/**
 * Request ID middleware for correlation
 */
export function withRequestId(handler: (requestId: string) => Promise<any>) {
  return async () => {
    const requestId = LoggerContext.getInstance().generateCorrelationId();
    const logger = new StructuredLogger();
    logger.setRequestId(requestId);
    
    return await handler(requestId);
  };
}

/**
 * Global logger instance
 */
export const logger = new StructuredLogger();

/**
 * Create component-specific logger
 */
export function createComponentLogger(component: string): StructuredLogger {
  const componentLogger = new StructuredLogger(`auth-token-manager.${component}`);
  componentLogger.context.setContext('component', { component });
  return componentLogger;
}

/**
 * Log configuration on startup
 */
export function logStartup(config: AppConfig): void {
  logger.info('Auth Token Manager starting up', {
    environment: config.environment,
    log_level: config.log_level,
    browser_type: config.browser.browser_type,
    browser_headless: config.browser.headless,
    k8s_namespace: config.k8s.namespace,
    monitoring_enabled: config.monitoring.enabled,
    component: 'startup'
  });
}

/**
 * Log shutdown
 */
export function logShutdown(reason: string): void {
  logger.info('Auth Token Manager shutting down', {
    reason,
    timestamp: new Date().toISOString(),
    component: 'shutdown'
  });
}
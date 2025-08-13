/**
 * Retry Utility with Exponential Backoff
 * 
 * This module provides robust retry mechanisms with exponential backoff,
 * jitter, and configurable retry conditions for resilient operations.
 */

import type { RetryConfig, AuthTokenError } from '../auth/types.ts';
import { logger } from './logger.ts';

/**
 * Retry operation result
 */
interface RetryResult<T> {
  success: boolean;
  result?: T;
  error?: Error;
  attempts: number;
  totalDuration: number;
}

/**
 * Retry context for tracking operation state
 */
interface RetryContext {
  attempt: number;
  totalDelay: number;
  startTime: number;
  lastError?: Error;
}

/**
 * Enhanced retry function with comprehensive error handling
 */
export async function retryWithBackoff<T>(
  operation: () => Promise<T>,
  config: RetryConfig
): Promise<T> {
  const context: RetryContext = {
    attempt: 0,
    totalDelay: 0,
    startTime: Date.now()
  };

  logger.debug('Starting retry operation', {
    max_attempts: config.max_attempts,
    initial_delay: config.initial_delay,
    max_delay: config.max_delay,
    backoff_factor: config.backoff_factor,
    jitter: config.jitter
  });

  while (context.attempt < config.max_attempts) {
    context.attempt++;

    try {
      const result = await operation();
      
      if (context.attempt > 1) {
        logger.info('Retry operation succeeded', {
          attempts: context.attempt,
          total_duration: Date.now() - context.startTime,
          total_delay: context.totalDelay
        });
      }

      return result;

    } catch (error) {
      context.lastError = error;

      const isLastAttempt = context.attempt >= config.max_attempts;
      const shouldRetry = !isLastAttempt && isRetryableError(error, config);

      logger.warn('Retry operation failed', {
        attempt: context.attempt,
        max_attempts: config.max_attempts,
        error: error.message,
        error_type: error.constructor.name,
        should_retry: shouldRetry,
        is_last_attempt: isLastAttempt
      });

      if (!shouldRetry) {
        const totalDuration = Date.now() - context.startTime;
        
        if (isLastAttempt) {
          logger.error('Retry operation exhausted all attempts', {
            attempts: context.attempt,
            total_duration: totalDuration,
            total_delay: context.totalDelay,
            final_error: error.message
          });
        } else {
          logger.error('Retry operation failed with non-retryable error', {
            attempts: context.attempt,
            total_duration: totalDuration,
            error: error.message,
            error_type: error.constructor.name
          });
        }

        throw error;
      }

      // Calculate delay for next attempt
      const delay = calculateDelay(context.attempt, config);
      context.totalDelay += delay;

      logger.debug('Delaying before retry', {
        attempt: context.attempt,
        delay_ms: delay,
        total_delay: context.totalDelay
      });

      await sleep(delay);
    }
  }

  // This should never be reached, but TypeScript requires it
  throw context.lastError || new Error('Retry operation failed unexpectedly');
}

/**
 * Retry with custom condition checker
 */
export async function retryWithCondition<T>(
  operation: () => Promise<T>,
  shouldRetry: (error: Error, attempt: number) => boolean,
  config: Partial<RetryConfig> = {}
): Promise<T> {
  const fullConfig: RetryConfig = {
    max_attempts: 3,
    initial_delay: 1000,
    max_delay: 10000,
    backoff_factor: 2,
    jitter: true,
    retryable_status_codes: [],
    retryable_errors: [],
    ...config
  };

  const context: RetryContext = {
    attempt: 0,
    totalDelay: 0,
    startTime: Date.now()
  };

  while (context.attempt < fullConfig.max_attempts) {
    context.attempt++;

    try {
      return await operation();
    } catch (error) {
      context.lastError = error;

      const isLastAttempt = context.attempt >= fullConfig.max_attempts;
      const customShouldRetry = !isLastAttempt && shouldRetry(error, context.attempt);

      if (!customShouldRetry) {
        throw error;
      }

      const delay = calculateDelay(context.attempt, fullConfig);
      context.totalDelay += delay;
      await sleep(delay);
    }
  }

  throw context.lastError || new Error('Retry operation failed');
}

/**
 * Retry with timeout
 */
export async function retryWithTimeout<T>(
  operation: () => Promise<T>,
  config: RetryConfig,
  timeoutMs: number
): Promise<T> {
  const timeoutPromise = new Promise<never>((_, reject) => {
    setTimeout(() => reject(new Error(`Operation timed out after ${timeoutMs}ms`)), timeoutMs);
  });

  return Promise.race([
    retryWithBackoff(operation, config),
    timeoutPromise
  ]);
}

/**
 * Circuit breaker with retry
 */
export class CircuitBreakerRetry<T> {
  private failureCount = 0;
  private lastFailureTime = 0;
  private state: 'closed' | 'open' | 'half-open' = 'closed';

  constructor(
    private operation: () => Promise<T>,
    private config: RetryConfig,
    private circuitConfig: {
      failureThreshold: number;
      recoveryTimeMs: number;
      successThreshold: number;
    }
  ) {}

  async execute(): Promise<T> {
    if (this.state === 'open') {
      if (Date.now() - this.lastFailureTime < this.circuitConfig.recoveryTimeMs) {
        throw new Error('Circuit breaker is open');
      }
      this.state = 'half-open';
    }

    try {
      const result = await retryWithBackoff(this.operation, this.config);
      
      if (this.state === 'half-open') {
        this.state = 'closed';
        this.failureCount = 0;
      }

      return result;

    } catch (error) {
      this.failureCount++;
      this.lastFailureTime = Date.now();

      if (this.failureCount >= this.circuitConfig.failureThreshold) {
        this.state = 'open';
        logger.warn('Circuit breaker opened', {
          failure_count: this.failureCount,
          threshold: this.circuitConfig.failureThreshold
        });
      }

      throw error;
    }
  }

  getState() {
    return {
      state: this.state,
      failureCount: this.failureCount,
      lastFailureTime: this.lastFailureTime
    };
  }

  reset() {
    this.state = 'closed';
    this.failureCount = 0;
    this.lastFailureTime = 0;
  }
}

/**
 * Batch retry for multiple operations
 */
export async function retryBatch<T>(
  operations: Array<() => Promise<T>>,
  config: RetryConfig,
  options: {
    concurrency?: number;
    failFast?: boolean;
  } = {}
): Promise<Array<RetryResult<T>>> {
  const { concurrency = 5, failFast = false } = options;
  const results: Array<RetryResult<T>> = [];

  // Process operations in batches
  for (let i = 0; i < operations.length; i += concurrency) {
    const batch = operations.slice(i, i + concurrency);
    
    const batchPromises = batch.map(async (operation, index) => {
      const startTime = Date.now();
      let attempts = 0;

      try {
        const result = await retryWithBackoff(operation, config);
        attempts = 1; // Will be updated by retry logic if needed

        return {
          success: true,
          result,
          attempts,
          totalDuration: Date.now() - startTime
        } as RetryResult<T>;

      } catch (error) {
        const result = {
          success: false,
          error: error as Error,
          attempts: config.max_attempts,
          totalDuration: Date.now() - startTime
        } as RetryResult<T>;

        if (failFast) {
          throw error;
        }

        return result;
      }
    });

    try {
      const batchResults = await Promise.all(batchPromises);
      results.push(...batchResults);
    } catch (error) {
      if (failFast) {
        throw error;
      }
    }
  }

  logger.info('Batch retry completed', {
    total_operations: operations.length,
    successful: results.filter(r => r.success).length,
    failed: results.filter(r => !r.success).length,
    concurrency
  });

  return results;
}

/**
 * Check if error is retryable based on configuration
 */
function isRetryableError(error: Error, config: RetryConfig): boolean {
  // Check for specific error types
  if (error.name && config.retryable_errors.includes(error.name.toLowerCase())) {
    return true;
  }

  // Check error message for retryable keywords
  const errorMessage = error.message.toLowerCase();
  for (const retryableError of config.retryable_errors) {
    if (errorMessage.includes(retryableError)) {
      return true;
    }
  }

  // Check for HTTP status codes if available
  if ('status' in error && typeof error.status === 'number') {
    return config.retryable_status_codes.includes(error.status);
  }

  // Check for auth token specific errors
  if (isAuthTokenError(error)) {
    return isRetryableAuthError(error);
  }

  // Check for network-related errors
  if (isNetworkError(error)) {
    return true;
  }

  // Default to not retryable
  return false;
}

/**
 * Check if error is an auth token error
 */
function isAuthTokenError(error: Error): error is AuthTokenError {
  return 'type' in error && typeof error.type === 'string';
}

/**
 * Check if auth token error is retryable
 */
function isRetryableAuthError(error: AuthTokenError): boolean {
  switch (error.type) {
    case 'browser_automation_error':
      return ['PAGE_LOAD_TIMEOUT', 'AUTOMATION_TIMEOUT'].includes(error.code);
    
    case 'kubernetes_error':
      return ['API_ERROR'].includes(error.code);
    
    case 'network_error':
      return ['CONNECTION_TIMEOUT', 'PROXY_ERROR'].includes(error.code);
    
    case 'oauth_flow_error':
      return false; // OAuth errors are generally not retryable
    
    case 'token_validation_error':
      return false; // Token validation errors are not retryable
    
    case 'configuration_error':
      return false; // Configuration errors are not retryable
    
    default:
      return false;
  }
}

/**
 * Check if error is a network-related error
 */
function isNetworkError(error: Error): boolean {
  const networkKeywords = [
    'timeout',
    'network',
    'connection',
    'dns',
    'socket',
    'fetch',
    'econnreset',
    'enotfound',
    'econnrefused'
  ];

  const errorMessage = error.message.toLowerCase();
  return networkKeywords.some(keyword => errorMessage.includes(keyword));
}

/**
 * Calculate delay with exponential backoff and jitter
 */
function calculateDelay(attempt: number, config: RetryConfig): number {
  // Exponential backoff: initial_delay * (backoff_factor ^ (attempt - 1))
  let delay = config.initial_delay * Math.pow(config.backoff_factor, attempt - 1);
  
  // Cap at max_delay
  delay = Math.min(delay, config.max_delay);
  
  // Add jitter if enabled
  if (config.jitter) {
    // Add random jitter of ±25%
    const jitterRange = delay * 0.25;
    const jitter = (Math.random() - 0.5) * 2 * jitterRange;
    delay += jitter;
  }
  
  // Ensure minimum delay
  return Math.max(delay, 0);
}

/**
 * Sleep utility function
 */
function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * Create a retry decorator for methods
 */
export function retryable(config: Partial<RetryConfig> = {}) {
  const fullConfig: RetryConfig = {
    max_attempts: 3,
    initial_delay: 1000,
    max_delay: 10000,
    backoff_factor: 2,
    jitter: true,
    retryable_status_codes: [429, 500, 502, 503, 504],
    retryable_errors: ['timeout', 'network'],
    ...config
  };

  return function (target: any, propertyKey: string, descriptor: PropertyDescriptor) {
    const originalMethod = descriptor.value;

    descriptor.value = async function (...args: any[]) {
      return retryWithBackoff(
        () => originalMethod.apply(this, args),
        fullConfig
      );
    };

    return descriptor;
  };
}

/**
 * Create exponential backoff utility for custom use cases
 */
export class ExponentialBackoff {
  private attempt = 0;

  constructor(private config: RetryConfig) {}

  async delay(): Promise<void> {
    this.attempt++;
    const delay = calculateDelay(this.attempt, this.config);
    await sleep(delay);
  }

  reset(): void {
    this.attempt = 0;
  }

  getAttempt(): number {
    return this.attempt;
  }

  hasAttemptsLeft(): boolean {
    return this.attempt < this.config.max_attempts;
  }
}
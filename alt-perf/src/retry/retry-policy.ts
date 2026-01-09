/**
 * Retry policy with exponential backoff
 * Provides configurable retry logic for handling transient failures
 */

/**
 * Retry policy configuration
 */
export interface RetryPolicy {
  /** Maximum number of attempts (including the first one) */
  maxAttempts: number;
  /** Base delay in milliseconds before first retry */
  baseDelayMs: number;
  /** Maximum delay in milliseconds (cap for exponential backoff) */
  maxDelayMs: number;
  /** Multiplier for exponential backoff */
  backoffMultiplier: number;
  /** Error types that should trigger a retry */
  retryableErrors: string[];
}

/**
 * Context passed to onRetry callback
 */
export interface RetryContext {
  /** Current attempt number (1-based, starts at 2 for first retry) */
  attempt: number;
  /** The error from the previous attempt */
  lastError: Error;
  /** Time when the operation started */
  startTime: number;
}

/**
 * Result of a retry operation
 */
export interface RetryResult<T> {
  /** Whether the operation succeeded */
  success: boolean;
  /** The result value if successful */
  value?: T;
  /** Number of attempts made */
  attempts: number;
  /** All errors encountered */
  errors: Error[];
  /** Total time spent in milliseconds */
  totalDuration: number;
}

/**
 * Default retry policy
 * - 3 attempts (1 initial + 2 retries)
 * - 1 second base delay with exponential backoff
 * - Retries common transient browser automation errors
 */
export const DEFAULT_RETRY_POLICY: RetryPolicy = {
  maxAttempts: 3,
  baseDelayMs: 1000,
  maxDelayMs: 10000,
  backoffMultiplier: 2,
  retryableErrors: [
    "TimeoutError",
    "NavigationError",
    "NetworkError",
    "ProtocolError",
  ],
};

/**
 * Retry executor with exponential backoff and jitter
 */
export class RetryExecutor<T> {
  constructor(private policy: RetryPolicy = DEFAULT_RETRY_POLICY) {}

  /**
   * Execute an operation with retry logic
   * @param operation The async operation to execute
   * @param onRetry Optional callback called before each retry
   * @returns RetryResult with success status and value or errors
   */
  async execute(
    operation: () => Promise<T>,
    onRetry?: (context: RetryContext) => void
  ): Promise<RetryResult<T>> {
    const startTime = performance.now();
    const errors: Error[] = [];
    let attempt = 0;

    while (attempt < this.policy.maxAttempts) {
      attempt++;

      try {
        const value = await operation();
        return {
          success: true,
          value,
          attempts: attempt,
          errors,
          totalDuration: performance.now() - startTime,
        };
      } catch (error) {
        const err = error instanceof Error ? error : new Error(String(error));
        errors.push(err);

        // Check if we should retry
        if (!this.isRetryable(err) || attempt >= this.policy.maxAttempts) {
          return {
            success: false,
            attempts: attempt,
            errors,
            totalDuration: performance.now() - startTime,
          };
        }

        // Call onRetry callback
        if (onRetry) {
          onRetry({
            attempt: attempt + 1,
            lastError: err,
            startTime,
          });
        }

        // Wait before retrying
        const delay = this.calculateDelay(attempt);
        await this.sleep(delay);
      }
    }

    // Should not reach here, but handle it anyway
    return {
      success: false,
      attempts: attempt,
      errors,
      totalDuration: performance.now() - startTime,
    };
  }

  /**
   * Check if an error is retryable based on the policy
   */
  private isRetryable(error: Error): boolean {
    const errorName = error.name;
    return this.policy.retryableErrors.some(
      (retryable) =>
        errorName === retryable ||
        errorName.includes(retryable) ||
        error.message.includes(retryable)
    );
  }

  /**
   * Calculate delay with exponential backoff and jitter
   */
  private calculateDelay(attempt: number): number {
    // Exponential backoff: baseDelay * (multiplier ^ (attempt - 1))
    const exponentialDelay =
      this.policy.baseDelayMs * Math.pow(this.policy.backoffMultiplier, attempt - 1);

    // Cap at maxDelay
    const cappedDelay = Math.min(exponentialDelay, this.policy.maxDelayMs);

    // Add jitter (0-25% of delay)
    const jitter = cappedDelay * Math.random() * 0.25;

    return cappedDelay + jitter;
  }

  /**
   * Sleep for a given number of milliseconds
   */
  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}

/**
 * Create a retry executor with the given policy
 */
export function createRetryExecutor<T>(
  policy: RetryPolicy = DEFAULT_RETRY_POLICY
): RetryExecutor<T> {
  return new RetryExecutor<T>(policy);
}

/**
 * Execute an operation with default retry policy
 * Convenience function for simple use cases
 */
export function withRetry<T>(
  operation: () => Promise<T>,
  policy?: Partial<RetryPolicy>
): Promise<RetryResult<T>> {
  const mergedPolicy = { ...DEFAULT_RETRY_POLICY, ...policy };
  const executor = new RetryExecutor<T>(mergedPolicy);
  return executor.execute(operation);
}

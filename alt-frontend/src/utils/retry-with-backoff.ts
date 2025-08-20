interface RetryConfig {
  maxRetries?: number;
  baseDelay?: number;
  maxDelay?: number;
  shouldRetry?: (error: unknown) => boolean;
}

export class RetryWithBackoff {
  private maxRetries: number;
  private baseDelay: number;
  private maxDelay: number;
  private shouldRetry: (error: unknown) => boolean;

  constructor(config: RetryConfig = {}) {
    this.maxRetries = config.maxRetries ?? 3;
    this.baseDelay = config.baseDelay ?? 1000;
    this.maxDelay = config.maxDelay ?? 8000;
    this.shouldRetry = config.shouldRetry ?? this.default401Retry;
  }

  private default401Retry(error: unknown): boolean {
    if (error instanceof Error) {
      return error.message.includes('401') || error.message.includes('unauthorized');
    }
    return false;
  }

  private calculateDelay(attempt: number): number {
    return Math.min(this.baseDelay * Math.pow(2, attempt), this.maxDelay);
  }

  async execute<T>(fn: () => Promise<T>): Promise<T> {
    let lastError: unknown;

    for (let attempt = 0; attempt <= this.maxRetries; attempt++) {
      try {
        return await fn();
      } catch (error) {
        lastError = error;

        // Don't retry if it's not a retryable error
        if (!this.shouldRetry(error)) {
          throw error;
        }

        // Don't wait after the last attempt
        if (attempt < this.maxRetries) {
          const delay = this.calculateDelay(attempt);
          console.warn(`[RETRY] Attempt ${attempt + 1} failed, retrying in ${delay}ms:`, error);
          await new Promise(resolve => setTimeout(resolve, delay));
        }
      }
    }

    console.error(`[RETRY] All ${this.maxRetries + 1} attempts failed`);
    throw lastError;
  }
}

// Singleton for 401 retries specifically
export const auth401Retry = new RetryWithBackoff({
  maxRetries: 2, // Total 3 attempts (initial + 2 retries)
  baseDelay: 2000, // Start with 2 second delay
  maxDelay: 8000,
  shouldRetry: (error) => {
    if (error instanceof Error) {
      return error.message.includes('401') || error.message.includes('unauthorized');
    }
    return false;
  }
});
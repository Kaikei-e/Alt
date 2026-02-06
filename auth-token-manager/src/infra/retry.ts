/**
 * Retry utility with exponential backoff
 */

import type { RetryConfig } from "../domain/types.ts";
import { logger } from "./logger.ts";

export async function retryWithBackoff<T>(
  operation: () => Promise<T>,
  config: RetryConfig,
  operationName = "operation",
): Promise<T> {
  let lastError: Error;

  for (let attempt = 1; attempt <= config.max_attempts; attempt++) {
    try {
      logger.debug(`Attempt ${attempt}/${config.max_attempts}`, {
        operation: operationName,
      });
      return await operation();
    } catch (error) {
      lastError = error instanceof Error ? error : new Error(String(error));

      if (attempt === config.max_attempts) {
        logger.error(`All ${config.max_attempts} attempts failed`, {
          operation: operationName,
        });
        throw lastError;
      }

      const delay = Math.min(
        config.base_delay *
          Math.pow(config.backoff_factor, attempt - 1),
        config.max_delay,
      );

      logger.warn(`Attempt ${attempt} failed, retrying in ${delay}ms`, {
        operation: operationName,
        next_delay_ms: delay,
      });
      await sleep(delay);
    }
  }

  throw lastError!;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

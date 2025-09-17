// 🚨 CRITICAL: X22 Phase 5 - CSRF Auto-Recovery System
// Automatic recovery and retry mechanism for CSRF errors

import { authAPI } from "@/lib/api/auth-client";

export interface RecoveryOptions {
  maxRetries?: number;
  retryDelay?: number;
  enableLogging?: boolean;
}

export interface RecoveryResult<T> {
  success: boolean;
  data?: T;
  error?: Error;
  attempts: number;
  totalTime: number;
  recoveryActions: string[];
}

export class CSRFAutoRecovery {
  private maxRetries: number;
  private retryDelay: number;
  private enableLogging: boolean;
  private activeRecoveries = new Map<string, Promise<unknown>>();

  constructor(options: RecoveryOptions = {}) {
    this.maxRetries = options.maxRetries ?? 3;
    this.retryDelay = options.retryDelay ?? 1000;
    this.enableLogging = options.enableLogging ?? true;
  }

  // 🚨 CRITICAL: Main auto-recovery method for authentication operations
  async executeWithRecovery<T>(
    operation: () => Promise<T>,
    context: string,
    options?: Partial<RecoveryOptions>,
  ): Promise<RecoveryResult<T>> {
    const startTime = performance.now();
    const maxRetries = options?.maxRetries ?? this.maxRetries;
    const retryDelay = options?.retryDelay ?? this.retryDelay;
    const recoveryActions: string[] = [];

    // Prevent concurrent recovery for the same context
    const existingRecovery = this.activeRecoveries.get(context);
    if (existingRecovery) {
      this.log(`🔄 Waiting for existing recovery in context: ${context}`);
      try {
        const result = await existingRecovery;
        return {
          success: true,
          data: result as T,
          attempts: 0,
          totalTime: performance.now() - startTime,
          recoveryActions: ["waited_for_existing_recovery"],
        };
      } catch (error) {
        // Continue with new recovery if existing one failed
      }
    }

    // Create new recovery promise
    const recoveryPromise = this.performRecovery(
      operation,
      context,
      maxRetries,
      retryDelay,
      recoveryActions,
    );

    this.activeRecoveries.set(context, recoveryPromise);

    try {
      const data = await recoveryPromise;
      const totalTime = performance.now() - startTime;

      this.log(
        `✅ Recovery successful for ${context} in ${totalTime.toFixed(2)}ms`,
      );

      return {
        success: true,
        data,
        attempts: recoveryActions.length,
        totalTime,
        recoveryActions,
      };
    } catch (error) {
      const totalTime = performance.now() - startTime;

      this.log(
        `❌ Recovery failed for ${context} after ${totalTime.toFixed(2)}ms:`,
        error,
      );

      return {
        success: false,
        error: error instanceof Error ? error : new Error(String(error)),
        attempts: recoveryActions.length,
        totalTime,
        recoveryActions,
      };
    } finally {
      this.activeRecoveries.delete(context);
    }
  }

  // 🚨 CRITICAL: Core recovery logic with progressive strategies
  private async performRecovery<T>(
    operation: () => Promise<T>,
    context: string,
    maxRetries: number,
    retryDelay: number,
    recoveryActions: string[],
  ): Promise<T> {
    for (let attempt = 0; attempt <= maxRetries; attempt++) {
      try {
        this.log(`🚀 Attempt ${attempt + 1}/${maxRetries + 1} for ${context}`);

        const result = await operation();

        if (attempt > 0) {
          recoveryActions.push(`successful_retry_attempt_${attempt + 1}`);
        }

        return result;
      } catch (error) {
        const isCSRFError = this.isCSRFError(error);
        const shouldRetry = isCSRFError && attempt < maxRetries;

        this.log(`❌ Attempt ${attempt + 1} failed for ${context}:`, {
          error: error instanceof Error ? error.message : String(error),
          isCSRFError,
          shouldRetry,
          attemptsRemaining: maxRetries - attempt,
        });

        if (shouldRetry) {
          // Progressive recovery strategies
          const recoveryStrategy = this.selectRecoveryStrategy(attempt, error);
          await this.executeRecoveryStrategy(
            recoveryStrategy,
            context,
            recoveryActions,
          );

          // Progressive delay (exponential backoff with jitter)
          const delay = this.calculateRetryDelay(attempt, retryDelay);
          await this.delay(delay);

          continue;
        }

        // No more retries available or non-CSRF error
        throw error;
      }
    }

    throw new Error(`Recovery failed after ${maxRetries + 1} attempts`);
  }

  // 🚨 CRITICAL: CSRF error detection with comprehensive patterns
  private isCSRFError(error: unknown): boolean {
    if (!(error instanceof Error)) return false;

    const message = error.message.toLowerCase();
    const csrfPatterns = [
      "csrf",
      "token",
      "forbidden",
      "unauthorized",
      "400",
      "401",
      "403",
      "500",
      "flow expired",
      "session",
      "cookie",
    ];

    return csrfPatterns.some((pattern) => message.includes(pattern));
  }

  // 🚨 CRITICAL: Progressive recovery strategy selection
  private selectRecoveryStrategy(
    attempt: number,
    error: unknown,
  ): RecoveryStrategy {
    const errorMessage =
      error instanceof Error ? error.message.toLowerCase() : "";

    // Strategy based on attempt number and error type
    if (attempt === 0) {
      // First retry: Simple refresh
      return "refresh_auth_flow";
    } else if (attempt === 1) {
      // Second retry: Clear and refresh
      return "clear_and_refresh";
    } else {
      // Final retry: Full reset
      return "full_reset";
    }
  }

  // 🚨 CRITICAL: Recovery strategy execution
  private async executeRecoveryStrategy(
    strategy: RecoveryStrategy,
    context: string,
    recoveryActions: string[],
  ): Promise<void> {
    this.log(`🔧 Executing recovery strategy: ${strategy} for ${context}`);

    try {
      switch (strategy) {
        case "refresh_auth_flow":
          await this.refreshAuthFlow(context);
          recoveryActions.push("refresh_auth_flow");
          break;

        case "clear_and_refresh":
          await this.clearAndRefreshAuth(context);
          recoveryActions.push("clear_and_refresh");
          break;

        case "full_reset":
          await this.fullAuthReset(context);
          recoveryActions.push("full_reset");
          break;

        default:
          this.log(`⚠️ Unknown recovery strategy: ${strategy}`);
          recoveryActions.push("unknown_strategy");
      }

      this.log(`✅ Recovery strategy ${strategy} executed successfully`);
    } catch (strategyError) {
      this.log(`❌ Recovery strategy ${strategy} failed:`, strategyError);
      recoveryActions.push(`${strategy}_failed`);
      // Don't throw - let the main retry loop continue
    }
  }

  // 🔧 Recovery Strategy: Refresh auth flow
  private async refreshAuthFlow(context: string): Promise<void> {
    if (context.includes("login")) {
      // Create new login flow
      await authAPI.initiateLogin();
    } else if (context.includes("register")) {
      // Create new registration flow
      await authAPI.initiateRegistration();
    }
    // Add small delay to let the new flow propagate
    await this.delay(500);
  }

  // 🔧 Recovery Strategy: Clear and refresh
  private async clearAndRefreshAuth(context: string): Promise<void> {
    // Clear any cached tokens or flows
    this.clearAuthCache();

    // Force new session check
    try {
      await authAPI.getCurrentUser();
    } catch {
      // Ignore errors, we're just trying to refresh
    }

    // Create new flow
    await this.refreshAuthFlow(context);
  }

  // 🔧 Recovery Strategy: Full reset
  private async fullAuthReset(context: string): Promise<void> {
    // Clear all auth state
    this.clearAuthCache();

    // Force logout to clear server session
    try {
      await authAPI.logout();
    } catch {
      // Ignore logout errors
    }

    // Wait longer for server cleanup
    await this.delay(2000);

    // Create completely new flow
    await this.refreshAuthFlow(context);
  }

  // 🧹 Clear client-side auth cache
  private clearAuthCache(): void {
    // Clear any cached auth data
    if (typeof window !== "undefined") {
      // Clear session storage
      sessionStorage.removeItem("auth_flow_id");
      sessionStorage.removeItem("csrf_token");

      // Clear any auth-related localStorage
      const authKeys = Object.keys(localStorage).filter(
        (key) =>
          key.includes("auth") || key.includes("csrf") || key.includes("token"),
      );
      authKeys.forEach((key) => localStorage.removeItem(key));
    }
  }

  // 🕰️ Calculate progressive retry delay
  private calculateRetryDelay(attempt: number, baseDelay: number): number {
    // Exponential backoff with jitter
    const exponentialDelay = baseDelay * Math.pow(2, attempt);
    const jitter = Math.random() * 500; // Add up to 500ms jitter
    return Math.min(exponentialDelay + jitter, 10000); // Cap at 10 seconds
  }

  // ⏳ Delay utility
  private delay(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  // 📝 Logging utility
  private log(message: string, data?: unknown): void {
    if (!this.enableLogging) return;

    const timestamp = new Date().toISOString();
    if (data) {
      console.log(`[CSRF-Recovery ${timestamp}] ${message}`, data);
    } else {
      console.log(`[CSRF-Recovery ${timestamp}] ${message}`);
    }
  }

  // 📊 Get recovery statistics
  getRecoveryStats(): RecoveryStats {
    return {
      activeRecoveries: this.activeRecoveries.size,
      configuration: {
        maxRetries: this.maxRetries,
        retryDelay: this.retryDelay,
        enableLogging: this.enableLogging,
      },
    };
  }

  // 🧹 Clear all active recoveries (for cleanup)
  clearActiveRecoveries(): void {
    this.activeRecoveries.clear();
  }
}

// Types
type RecoveryStrategy =
  | "refresh_auth_flow"
  | "clear_and_refresh"
  | "full_reset";

interface RecoveryStats {
  activeRecoveries: number;
  configuration: {
    maxRetries: number;
    retryDelay: number;
    enableLogging: boolean;
  };
}

// Export singleton instance
export const csrfAutoRecovery = new CSRFAutoRecovery({
  maxRetries: 3,
  retryDelay: 1000,
  enableLogging: true,
});

// 🚨 CRITICAL: Convenience methods for common operations
export const withAutoRecovery = {
  // Auto-recovery for login
  async login(email: string, password: string): Promise<RecoveryResult<any>> {
    return csrfAutoRecovery.executeWithRecovery(
      async () => {
        const flow = await authAPI.initiateLogin();
        return authAPI.completeLogin(flow.id, email, password);
      },
      "login",
      { maxRetries: 2 },
    );
  },

  // Auto-recovery for registration
  async register(
    email: string,
    password: string,
    name?: string,
  ): Promise<RecoveryResult<any>> {
    return csrfAutoRecovery.executeWithRecovery(
      async () => {
        const flow = await authAPI.initiateRegistration();
        return authAPI.completeRegistration(flow.id, email, password, name);
      },
      "registration",
      { maxRetries: 2 },
    );
  },

  // Auto-recovery for any auth operation
  async execute<T>(
    operation: () => Promise<T>,
    context: string,
  ): Promise<RecoveryResult<T>> {
    return csrfAutoRecovery.executeWithRecovery(operation, context);
  },
};

import { ApiClientError } from "@/lib/api";

export type ErrorSeverity = "low" | "medium" | "high" | "critical";

export interface ErrorInfo {
  message: string;
  severity: ErrorSeverity;
  code?: string;
  context?: Record<string, unknown>;
}

export class ErrorHandler {
  private static logError(error: ErrorInfo): void {
    const logLevel =
      error.severity === "critical"
        ? "error"
        : error.severity === "high"
          ? "error"
          : error.severity === "medium"
            ? "warn"
            : "log";

    console[logLevel](`[${error.severity.toUpperCase()}] ${error.message}`, {
      code: error.code,
      context: error.context,
    });
  }

  static handleApiError(
    error: unknown,
    context?: Record<string, unknown>,
  ): ErrorInfo {
    const errorInfo = ErrorHandler.createErrorInfo(error, context);
    ErrorHandler.logError(errorInfo);
    return errorInfo;
  }

  private static createErrorInfo(
    error: unknown,
    context?: Record<string, unknown>,
  ): ErrorInfo {
    if (error instanceof ApiClientError) {
      return {
        message: error.message,
        severity: ErrorHandler.getSeverityFromStatus(error.status),
        code: error.code || error.status?.toString(),
        context,
      };
    }

    if (error instanceof Error) {
      return {
        message: error.message,
        severity: "medium",
        context,
      };
    }

    return {
      message: "An unknown error occurred",
      severity: "medium",
      context,
    };
  }

  private static getSeverityFromStatus(status?: number): ErrorSeverity {
    if (!status) return "medium";
    if (status >= 500) return "high";
    if (status >= 400) return "medium";
    return "low";
  }

  static handleValidationError(
    error: unknown,
    context?: Record<string, unknown>,
  ): ErrorInfo {
    const errorInfo: ErrorInfo = {
      message: error instanceof Error ? error.message : "Validation failed",
      severity: "low",
      code: "VALIDATION_ERROR",
      context,
    };

    ErrorHandler.logError(errorInfo);
    return errorInfo;
  }

  static handleNetworkError(
    _error: unknown,
    context?: Record<string, unknown>,
  ): ErrorInfo {
    const errorInfo: ErrorInfo = {
      message: "Network error occurred. Please check your connection.",
      severity: "medium",
      code: "NETWORK_ERROR",
      context,
    };

    ErrorHandler.logError(errorInfo);
    return errorInfo;
  }

  static createUserFriendlyMessage(error: ErrorInfo): string {
    if (error.severity === "critical") {
      return "A critical error occurred. Please refresh the page.";
    }

    const messageMap: Record<string, string> = {
      VALIDATION_ERROR: error.message,
      NETWORK_ERROR:
        "Unable to connect. Please check your internet connection.",
      "404": "The requested resource was not found.",
      "500": "Server error. Please try again later.",
      "408": "Request timeout. Please try again.",
    };

    return messageMap[error.code || ""] || error.message;
  }
}

import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { ErrorHandler } from "@/lib/utils/errorHandler";
import { ApiClientError } from "@/lib/api";

describe("ErrorHandler", () => {
  let consoleSpy: {
    log: ReturnType<typeof vi.spyOn>;
    warn: ReturnType<typeof vi.spyOn>;
    error: ReturnType<typeof vi.spyOn>;
  };

  beforeEach(() => {
    consoleSpy = {
      log: vi.spyOn(console, "log").mockImplementation(() => {}),
      warn: vi.spyOn(console, "warn").mockImplementation(() => {}),
      error: vi.spyOn(console, "error").mockImplementation(() => {}),
    };
  });

  afterEach(() => {
    Object.values(consoleSpy).forEach((spy) => spy.mockRestore());
  });

  describe("handleApiError", () => {
    it("should handle ApiClientError with status code", () => {
      const apiError = new ApiClientError(
        "Server error",
        500,
        "INTERNAL_ERROR",
      );
      const result = ErrorHandler.handleApiError(apiError, { userId: "123" });

      expect(result).toEqual({
        message: "Server error",
        severity: "high",
        code: "INTERNAL_ERROR",
        context: { userId: "123" },
      });

      expect(consoleSpy.error).toHaveBeenCalledWith("[HIGH] Server error", {
        code: "INTERNAL_ERROR",
        context: { userId: "123" },
      });
    });

    it("should handle ApiClientError with client error status", () => {
      const apiError = new ApiClientError("Not found", 404);
      const result = ErrorHandler.handleApiError(apiError);

      expect(result).toEqual({
        message: "Not found",
        severity: "medium",
        code: "404",
        context: undefined,
      });

      expect(consoleSpy.warn).toHaveBeenCalledWith("[MEDIUM] Not found", {
        code: "404",
        context: undefined,
      });
    });

    it("should handle generic Error", () => {
      const error = new Error("Generic error");
      const result = ErrorHandler.handleApiError(error, {
        action: "fetchData",
      });

      expect(result).toEqual({
        message: "Generic error",
        severity: "medium",
        context: { action: "fetchData" },
      });

      expect(consoleSpy.warn).toHaveBeenCalledWith("[MEDIUM] Generic error", {
        code: undefined,
        context: { action: "fetchData" },
      });
    });

    it("should handle unknown error types", () => {
      const result = ErrorHandler.handleApiError("string error");

      expect(result).toEqual({
        message: "An unknown error occurred",
        severity: "medium",
        context: undefined,
      });

      expect(consoleSpy.warn).toHaveBeenCalledWith(
        "[MEDIUM] An unknown error occurred",
        { code: undefined, context: undefined },
      );
    });

    it("should handle null or undefined errors", () => {
      const nullResult = ErrorHandler.handleApiError(null);
      const undefinedResult = ErrorHandler.handleApiError(undefined);

      expect(nullResult.message).toBe("An unknown error occurred");
      expect(undefinedResult.message).toBe("An unknown error occurred");
    });
  });

  describe("handleValidationError", () => {
    it("should handle validation error with custom message", () => {
      const error = new Error("Field is required");
      const result = ErrorHandler.handleValidationError(error, {
        field: "email",
      });

      expect(result).toEqual({
        message: "Field is required",
        severity: "low",
        code: "VALIDATION_ERROR",
        context: { field: "email" },
      });

      expect(consoleSpy.log).toHaveBeenCalledWith("[LOW] Field is required", {
        code: "VALIDATION_ERROR",
        context: { field: "email" },
      });
    });

    it("should handle non-Error validation failures", () => {
      const result = ErrorHandler.handleValidationError("validation failed");

      expect(result).toEqual({
        message: "Validation failed",
        severity: "low",
        code: "VALIDATION_ERROR",
        context: undefined,
      });
    });
  });

  describe("handleNetworkError", () => {
    it("should handle network errors", () => {
      const networkError = new Error("Failed to fetch");
      const result = ErrorHandler.handleNetworkError(networkError, {
        url: "/api/data",
      });

      expect(result).toEqual({
        message: "Network error occurred. Please check your connection.",
        severity: "medium",
        code: "NETWORK_ERROR",
        context: { url: "/api/data" },
      });

      expect(consoleSpy.warn).toHaveBeenCalledWith(
        "[MEDIUM] Network error occurred. Please check your connection.",
        { code: "NETWORK_ERROR", context: { url: "/api/data" } },
      );
    });
  });

  describe("createUserFriendlyMessage", () => {
    it("should return appropriate messages for different error codes", () => {
      const testCases = [
        {
          error: {
            message: "Field required",
            severity: "low" as const,
            code: "VALIDATION_ERROR",
          },
          expected: "Field required",
        },
        {
          error: {
            message: "Network failed",
            severity: "medium" as const,
            code: "NETWORK_ERROR",
          },
          expected: "Unable to connect. Please check your internet connection.",
        },
        {
          error: {
            message: "Not found",
            severity: "medium" as const,
            code: "404",
          },
          expected: "The requested resource was not found.",
        },
        {
          error: {
            message: "Server error",
            severity: "high" as const,
            code: "500",
          },
          expected: "Server error. Please try again later.",
        },
        {
          error: {
            message: "Timeout",
            severity: "medium" as const,
            code: "408",
          },
          expected: "Request timeout. Please try again.",
        },
        {
          error: {
            message: "Critical failure",
            severity: "critical" as const,
            code: "UNKNOWN",
          },
          expected: "A critical error occurred. Please refresh the page.",
        },
        {
          error: { message: "Some error", severity: "medium" as const },
          expected: "Some error",
        },
      ];

      testCases.forEach(({ error, expected }) => {
        const result = ErrorHandler.createUserFriendlyMessage(error);
        expect(result).toBe(expected);
      });
    });

    it("should handle missing code gracefully", () => {
      const error = { message: "Unknown error", severity: "medium" as const };
      const result = ErrorHandler.createUserFriendlyMessage(error);
      expect(result).toBe("Unknown error");
    });

    it("should prioritize critical severity over error code", () => {
      const error = {
        message: "Database connection lost",
        severity: "critical" as const,
        code: "404",
      };
      const result = ErrorHandler.createUserFriendlyMessage(error);
      expect(result).toBe(
        "A critical error occurred. Please refresh the page.",
      );
    });
  });

  describe("logging behavior", () => {
    it("should use correct log levels for different severities", () => {
      ErrorHandler.handleApiError(new ApiClientError("Critical", 500), {
        test: true,
      });
      expect(consoleSpy.error).toHaveBeenCalled();

      ErrorHandler.handleApiError(new ApiClientError("High", 503), {
        test: true,
      });
      expect(consoleSpy.error).toHaveBeenCalled();

      ErrorHandler.handleApiError(new ApiClientError("Medium", 400), {
        test: true,
      });
      expect(consoleSpy.warn).toHaveBeenCalled();

      ErrorHandler.handleValidationError(new Error("Low"), { test: true });
      expect(consoleSpy.log).toHaveBeenCalled();
    });

    it("should include context information in logs", () => {
      const context = {
        userId: "user123",
        action: "updateProfile",
        timestamp: Date.now(),
      };

      ErrorHandler.handleApiError(new Error("Test error"), context);

      expect(consoleSpy.warn).toHaveBeenCalledWith(
        expect.stringContaining("Test error"),
        expect.objectContaining({ context }),
      );
    });
  });
});

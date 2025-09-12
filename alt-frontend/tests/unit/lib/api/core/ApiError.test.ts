import { describe, it, expect } from "vitest";
import { ApiError } from "../../../../../src/lib/api/core/ApiError";

describe("ApiError", () => {
  it("should create error with message only", () => {
    const error = new ApiError("Test error");

    expect(error.message).toBe("Test error");
    expect(error.name).toBe("ApiError");
    expect(error.status).toBeUndefined();
    expect(error.code).toBeUndefined();
  });

  it("should create error with message and status", () => {
    const error = new ApiError("Test error", 404);

    expect(error.message).toBe("Test error");
    expect(error.name).toBe("ApiError");
    expect(error.status).toBe(404);
    expect(error.code).toBeUndefined();
  });

  it("should create error with message, status, and code", () => {
    const error = new ApiError("Test error", 400, "INVALID_REQUEST");

    expect(error.message).toBe("Test error");
    expect(error.name).toBe("ApiError");
    expect(error.status).toBe(400);
    expect(error.code).toBe("INVALID_REQUEST");
  });

  it("should be instanceof Error", () => {
    const error = new ApiError("Test error");

    expect(error).toBeInstanceOf(Error);
    expect(error).toBeInstanceOf(ApiError);
  });

  it("should maintain error stack trace", () => {
    const error = new ApiError("Test error");

    expect(error.stack).toBeDefined();
    expect(error.stack).toContain("ApiError");
  });

  it("should handle undefined status and code", () => {
    const error = new ApiError("Test error", undefined, undefined);

    expect(error.message).toBe("Test error");
    expect(error.status).toBeUndefined();
    expect(error.code).toBeUndefined();
  });

  it("should create timeout error", () => {
    const error = new ApiError("Request timeout", 408);

    expect(error.message).toBe("Request timeout");
    expect(error.status).toBe(408);
  });

  it("should create unauthorized error", () => {
    const error = new ApiError("Unauthorized", 401, "AUTH_REQUIRED");

    expect(error.message).toBe("Unauthorized");
    expect(error.status).toBe(401);
    expect(error.code).toBe("AUTH_REQUIRED");
  });
});

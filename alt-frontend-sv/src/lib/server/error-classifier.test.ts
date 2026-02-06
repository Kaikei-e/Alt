import { describe, expect, it } from "vitest";
import { classifyOryError } from "./error-classifier";

describe("classifyOryError", () => {
	it("should default to 401 for generic errors", () => {
		const result = classifyOryError(new Error("session expired"));
		expect(result.status).toBe(401);
		expect(result.message).toBe("session expired");
	});

	it("should extract statusCode from ApiError-like object", () => {
		const error = { statusCode: 403, message: "Forbidden", name: "ApiError" };
		const result = classifyOryError(error);
		expect(result.status).toBe(403);
		expect(result.safeLogInfo.name).toBe("ApiError");
		expect(result.safeLogInfo.statusCode).toBe(403);
	});

	it("should extract status from response.status", () => {
		const error = {
			message: "Request failed",
			response: { status: 500, statusText: "Internal Server Error" },
		};
		const result = classifyOryError(error);
		expect(result.status).toBe(500);
		expect(result.safeLogInfo.responseStatus).toBe(500);
		expect(result.safeLogInfo.responseStatusText).toBe("Internal Server Error");
	});

	it("should detect 403 from error message", () => {
		const result = classifyOryError(new Error("403 Forbidden"));
		expect(result.status).toBe(403);
	});

	it("should detect Forbidden from error message", () => {
		const result = classifyOryError(new Error("Access Forbidden"));
		expect(result.status).toBe(403);
	});

	it("should truncate long messages to 200 chars", () => {
		const longMessage = "x".repeat(300);
		const result = classifyOryError(new Error(longMessage));
		expect(result.message.length).toBe(200);
	});

	it("should handle non-Error, non-object values", () => {
		const result = classifyOryError("string error");
		expect(result.status).toBe(401);
		expect(result.message).toBe("string error");
		expect(result.safeLogInfo).toEqual({});
	});

	it("should truncate response data to 500 chars", () => {
		const error = {
			message: "fail",
			response: {
				status: 400,
				data: { detail: "a".repeat(600) },
			},
		};
		const result = classifyOryError(error);
		expect(result.safeLogInfo.responseData).toBeDefined();
		expect(
			(result.safeLogInfo.responseData as string).length,
		).toBeLessThanOrEqual(500);
	});
});

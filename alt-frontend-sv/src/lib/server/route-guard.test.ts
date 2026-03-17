import { describe, expect, it } from "vitest";
import { isPublicRoute, isApiRoute, isStreamEndpoint } from "./route-guard";

describe("route-guard", () => {
	describe("isPublicRoute", () => {
		it.each([
			"/login",
			"/register",
			"/auth/callback",
			"/health",
			"/logout",
			"/recovery",
			"/verification",
			"/error",
			"/public/landing",
			"/landing",
			"/favicon.ico",
			"/icon.svg",
			"/test",
			"/test/something",
		])("should return true for public route: %s", (pathname) => {
			expect(isPublicRoute(pathname)).toBe(true);
		});

		it.each([
			"/feeds",
			"/desktop/feeds",
			"/api/articles",
			"/mobile/recap",
		])("should return false for protected route: %s", (pathname) => {
			expect(isPublicRoute(pathname)).toBe(false);
		});
	});

	describe("isApiRoute", () => {
		it("should return true for /api/ paths", () => {
			expect(isApiRoute("/api/articles")).toBe(true);
		});

		it("should return true for /api/health paths", () => {
			expect(isApiRoute("/api/health")).toBe(true);
		});

		it("should return false for non-API paths", () => {
			expect(isApiRoute("/feeds")).toBe(false);
		});
	});

	describe("isStreamEndpoint", () => {
		it("should return true for /stream paths", () => {
			expect(isStreamEndpoint("/api/stream")).toBe(true);
		});

		it("should return true for /sse paths", () => {
			expect(isStreamEndpoint("/api/sse")).toBe(true);
		});

		it("should return false for non-stream paths", () => {
			expect(isStreamEndpoint("/api/articles")).toBe(false);
		});
	});
});

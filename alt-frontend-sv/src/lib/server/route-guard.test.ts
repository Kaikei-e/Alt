import { describe, expect, it } from "vitest";
import { isPublicRoute, isApiRoute, isStreamEndpoint } from "./route-guard";

describe("route-guard", () => {
	describe("isPublicRoute", () => {
		it.each([
			"/sv/login",
			"/sv/register",
			"/sv/auth/callback",
			"/sv/health",
			"/sv/logout",
			"/sv/recovery",
			"/sv/verification",
			"/sv/error",
			"/sv/public/landing",
			"/sv/landing",
			"/favicon.ico",
			"/icon.svg",
			"/sv/test",
			"/sv/test/something",
		])("should return true for public route: %s", (pathname) => {
			expect(isPublicRoute(pathname)).toBe(true);
		});

		it.each([
			"/sv/home",
			"/sv/desktop/feeds",
			"/sv/api/articles",
			"/sv/mobile/recap",
		])("should return false for protected route: %s", (pathname) => {
			expect(isPublicRoute(pathname)).toBe(false);
		});
	});

	describe("isApiRoute", () => {
		it("should return true for /sv/api/ paths", () => {
			expect(isApiRoute("/sv/api/articles")).toBe(true);
		});

		it("should return true for /api/ paths", () => {
			expect(isApiRoute("/api/health")).toBe(true);
		});

		it("should return false for non-API paths", () => {
			expect(isApiRoute("/sv/home")).toBe(false);
		});
	});

	describe("isStreamEndpoint", () => {
		it("should return true for /stream paths", () => {
			expect(isStreamEndpoint("/sv/api/stream")).toBe(true);
		});

		it("should return true for /sse paths", () => {
			expect(isStreamEndpoint("/sv/api/sse")).toBe(true);
		});

		it("should return false for non-stream paths", () => {
			expect(isStreamEndpoint("/sv/api/articles")).toBe(false);
		});
	});
});

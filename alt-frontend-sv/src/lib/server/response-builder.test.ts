import { describe, expect, it } from "vitest";
import { buildApiErrorResponse, buildRedirectUrl } from "./response-builder";

describe("buildApiErrorResponse", () => {
	it("should return 401 JSON response for authentication errors", async () => {
		const response = buildApiErrorResponse({
			status: 401,
			isStreamEndpoint: false,
		});
		expect(response.status).toBe(401);
		const body = await response.json();
		expect(body.error).toBe("Authentication required");
		expect(body.message).toBe("Session validation failed");
		expect(response.headers.get("Content-Type")).toBe("application/json");
	});

	it("should return 403 JSON response for forbidden errors", async () => {
		const response = buildApiErrorResponse({
			status: 403,
			isStreamEndpoint: false,
		});
		expect(response.status).toBe(403);
		const body = await response.json();
		expect(body.error).toBe("Forbidden");
	});

	it("should add streaming headers for SSE endpoints", () => {
		const response = buildApiErrorResponse({
			status: 401,
			isStreamEndpoint: true,
		});
		expect(response.headers.get("X-Accel-Buffering")).toBe("no");
		expect(response.headers.get("Connection")).toBe("close");
	});

	it("should not add streaming headers for non-SSE endpoints", () => {
		const response = buildApiErrorResponse({
			status: 401,
			isStreamEndpoint: false,
		});
		expect(response.headers.get("X-Accel-Buffering")).toBeNull();
	});
});

describe("buildRedirectUrl", () => {
	it("should redirect /sv to /sv/home", () => {
		const url = buildRedirectUrl("/sv", "http://localhost:3000");
		expect(url).toBe(
			`/sv/login?return_to=${encodeURIComponent("http://localhost:3000/sv/home")}`,
		);
	});

	it("should redirect /sv/ to /sv/home", () => {
		const url = buildRedirectUrl("/sv/", "http://localhost:3000");
		expect(url).toBe(
			`/sv/login?return_to=${encodeURIComponent("http://localhost:3000/sv/home")}`,
		);
	});

	it("should redirect arbitrary paths with encoded pathname", () => {
		const url = buildRedirectUrl("/sv/desktop/feeds", "http://localhost:3000");
		expect(url).toBe(
			`/sv/login?return_to=${encodeURIComponent("/sv/desktop/feeds")}`,
		);
	});
});

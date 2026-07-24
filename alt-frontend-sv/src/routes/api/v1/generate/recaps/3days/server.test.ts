import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$env/dynamic/private", () => ({
	env: { RECAP_WORKER_BASE_URL: "http://recap-worker.test" },
}));

const { getBackendToken, getCSRFToken } = vi.hoisted(() => ({
	getBackendToken: vi.fn(),
	getCSRFToken: vi.fn(),
}));

vi.mock("$lib/api", () => ({ getBackendToken, getCSRFToken }));

import { POST } from "./+server";

function makeEvent(body: unknown, csrfHeader?: string) {
	const headers = new Headers({ "Content-Type": "application/json" });
	if (csrfHeader !== undefined) headers.set("X-CSRF-Token", csrfHeader);
	headers.set("cookie", "ory_kratos_session=abc");
	return {
		request: new Request("http://localhost/api/v1/generate/recaps/3days", {
			method: "POST",
			headers,
			body: JSON.stringify(body),
		}),
	} as unknown as Parameters<typeof POST>[0];
}

describe("POST /api/v1/generate/recaps/3days — CSRF", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		getBackendToken.mockResolvedValue("backend-token");
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(
				new Response(JSON.stringify({ job_id: "1" }), { status: 200 }),
			),
		);
	});

	it("rejects the request with 403 when no X-CSRF-Token header is present", async () => {
		getCSRFToken.mockResolvedValue("expected-token");

		const res = await POST(makeEvent({}));

		expect(res.status).toBe(403);
		expect(fetch).not.toHaveBeenCalled();
	});

	it("rejects the request with 403 when X-CSRF-Token does not match the session token", async () => {
		getCSRFToken.mockResolvedValue("expected-token");

		const res = await POST(makeEvent({}, "wrong-token"));

		expect(res.status).toBe(403);
		expect(fetch).not.toHaveBeenCalled();
	});

	it("proceeds when X-CSRF-Token matches the session token", async () => {
		getCSRFToken.mockResolvedValue("expected-token");

		const res = await POST(makeEvent({}, "expected-token"));

		expect(res.status).toBe(200);
		expect(fetch).toHaveBeenCalledTimes(1);
	});
});

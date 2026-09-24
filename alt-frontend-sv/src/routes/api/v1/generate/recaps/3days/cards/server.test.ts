import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$env/dynamic/private", () => ({
	env: { RECAP_WORKER_BASE_URL: "http://recap-worker.test" },
}));

const { getBackendToken } = vi.hoisted(() => ({
	getBackendToken: vi.fn(),
}));

vi.mock("$lib/api", async (importOriginal) => {
	const actual = await importOriginal<typeof import("$lib/api")>();
	return { ...actual, getBackendToken };
});

import { POST } from "./+server";

function makeEvent(
	body: unknown,
	opts: { csrfHeader?: string; cookieCsrf?: string } = {},
) {
	const csrfHeader = opts.csrfHeader;
	const cookieCsrf = "cookieCsrf" in opts ? opts.cookieCsrf : "expected-token";
	const headers = new Headers({ "Content-Type": "application/json" });
	if (csrfHeader !== undefined) headers.set("X-CSRF-Token", csrfHeader);
	headers.set("cookie", "ory_kratos_session=abc");
	return {
		request: new Request(
			"http://localhost/api/v1/generate/recaps/3days/cards",
			{
				method: "POST",
				headers,
				body: JSON.stringify(body),
			},
		),
		cookies: {
			get: (name: string) => (name === "csrf_token" ? cookieCsrf : undefined),
		},
	} as unknown as Parameters<typeof POST>[0];
}

describe("POST /api/v1/generate/recaps/3days/cards", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		getBackendToken.mockResolvedValue("backend-token");
		vi.stubGlobal(
			"fetch",
			vi
				.fn()
				.mockResolvedValue(
					new Response(JSON.stringify({ job_id: "cards-1" }), { status: 202 }),
				),
		);
	});

	it("rejects the request with 403 when no X-CSRF-Token header is present", async () => {
		const res = await POST(makeEvent({}));

		expect(res.status).toBe(403);
		expect(fetch).not.toHaveBeenCalled();
	});

	it("rejects the request with 403 when X-CSRF-Token does not match the double-submit cookie", async () => {
		const res = await POST(makeEvent({}, { csrfHeader: "wrong-token" }));

		expect(res.status).toBe(403);
		expect(fetch).not.toHaveBeenCalled();
	});

	it("rejects the request with 403 when no csrf cookie was ever issued", async () => {
		const res = await POST(
			makeEvent({}, { csrfHeader: "expected-token", cookieCsrf: undefined }),
		);

		expect(res.status).toBe(403);
		expect(fetch).not.toHaveBeenCalled();
	});

	it("forwards to recap-worker /v1/generate/recaps/3days/cards on happy path", async () => {
		const res = await POST(makeEvent({}, { csrfHeader: "expected-token" }));

		expect(res.status).toBe(202);
		expect(fetch).toHaveBeenCalledTimes(1);
		const call = vi.mocked(fetch).mock.calls[0];
		expect(call?.[0]).toBe(
			"http://recap-worker.test/v1/generate/recaps/3days/cards",
		);
		const body = await res.json();
		expect(body).toEqual({ job_id: "cards-1" });
	});

	it("passes through 409 status and JSON error when job is already running", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(
				new Response(
					JSON.stringify({ error: "Cards recap job already running" }),
					{
						status: 409,
						headers: { "Content-Type": "application/json" },
					},
				),
			),
		);

		const res = await POST(makeEvent({}, { csrfHeader: "expected-token" }));

		expect(res.status).toBe(409);
		const body = await res.json();
		expect(body.error).toBe("Cards recap job already running");
	});

	it("passes through 503 status and JSON error when user is not configured", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(
				new Response(
					JSON.stringify({ error: "Cards recap user not configured" }),
					{
						status: 503,
						headers: { "Content-Type": "application/json" },
					},
				),
			),
		);

		const res = await POST(makeEvent({}, { csrfHeader: "expected-token" }));

		expect(res.status).toBe(503);
		const body = await res.json();
		expect(body.error).toBe("Cards recap user not configured");
	});
});

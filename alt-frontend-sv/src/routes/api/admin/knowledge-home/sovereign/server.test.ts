import { beforeEach, describe, expect, it, vi } from "vitest";

const { createSovereignSnapshot, runSovereignRetention } = vi.hoisted(() => ({
	createSovereignSnapshot: vi.fn(),
	runSovereignRetention: vi.fn(),
}));

// Real verifyCsrfToken runs so this is an end-to-end test of the route's
// double-submit-cookie guard, not just a mocked comparison.
vi.mock("$lib/api", async (importOriginal) => {
	const actual = await importOriginal<typeof import("$lib/api")>();
	return { ...actual };
});
vi.mock("$lib/server/sovereign-admin", () => ({
	fetchSovereignAdminSnapshot: vi.fn(),
	createSovereignSnapshot,
	runSovereignRetention,
}));

import { POST } from "./+server";

function makeEvent(
	body: unknown,
	opts: { csrfHeader?: string; cookieCsrf?: string } = {},
) {
	const csrfHeader = opts.csrfHeader;
	const cookieCsrf = "cookieCsrf" in opts ? opts.cookieCsrf : "expected-token";
	const headers = new Headers({ "Content-Type": "application/json" });
	if (csrfHeader !== undefined) headers.set("X-CSRF-Token", csrfHeader);
	return {
		request: new Request(
			"http://localhost/api/admin/knowledge-home/sovereign",
			{
				method: "POST",
				headers,
				body: JSON.stringify(body),
			},
		),
		locals: { user: { traits: { role: "admin" } } },
		cookies: {
			get: (name: string) => (name === "csrf_token" ? cookieCsrf : undefined),
		},
	} as unknown as Parameters<typeof POST>[0];
}

describe("POST /api/admin/knowledge-home/sovereign — CSRF", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		createSovereignSnapshot.mockResolvedValue({ ok: true });
	});

	it("rejects the request with 403 when no X-CSRF-Token header is present", async () => {
		const res = await POST(makeEvent({ action: "create_snapshot" }));

		expect(res.status).toBe(403);
		expect(createSovereignSnapshot).not.toHaveBeenCalled();
	});

	it("rejects the request with 403 when X-CSRF-Token does not match the double-submit cookie", async () => {
		const res = await POST(
			makeEvent({ action: "create_snapshot" }, { csrfHeader: "wrong-token" }),
		);

		expect(res.status).toBe(403);
		expect(createSovereignSnapshot).not.toHaveBeenCalled();
	});

	it("rejects the request with 403 when no csrf cookie was ever issued", async () => {
		const res = await POST(
			makeEvent(
				{ action: "create_snapshot" },
				{ csrfHeader: "expected-token", cookieCsrf: undefined },
			),
		);

		expect(res.status).toBe(403);
		expect(createSovereignSnapshot).not.toHaveBeenCalled();
	});

	it("proceeds when X-CSRF-Token matches the double-submit cookie", async () => {
		const res = await POST(
			makeEvent(
				{ action: "create_snapshot" },
				{ csrfHeader: "expected-token" },
			),
		);

		expect(res.status).toBe(200);
		expect(createSovereignSnapshot).toHaveBeenCalledTimes(1);
	});
});

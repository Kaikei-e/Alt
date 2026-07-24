import { beforeEach, describe, expect, it, vi } from "vitest";

const { getCSRFToken, createSovereignSnapshot, runSovereignRetention } =
	vi.hoisted(() => ({
		getCSRFToken: vi.fn(),
		createSovereignSnapshot: vi.fn(),
		runSovereignRetention: vi.fn(),
	}));

vi.mock("$lib/api", () => ({ getCSRFToken }));
vi.mock("$lib/server/sovereign-admin", () => ({
	fetchSovereignAdminSnapshot: vi.fn(),
	createSovereignSnapshot,
	runSovereignRetention,
}));

import { POST } from "./+server";

function makeEvent(body: unknown, csrfHeader?: string) {
	const headers = new Headers({ "Content-Type": "application/json" });
	if (csrfHeader !== undefined) headers.set("X-CSRF-Token", csrfHeader);
	return {
		request: new Request("http://localhost/api/admin/knowledge-home/sovereign", {
			method: "POST",
			headers,
			body: JSON.stringify(body),
		}),
		locals: { user: { traits: { role: "admin" } } },
	} as unknown as Parameters<typeof POST>[0];
}

describe("POST /api/admin/knowledge-home/sovereign — CSRF", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		createSovereignSnapshot.mockResolvedValue({ ok: true });
	});

	it("rejects the request with 403 when no X-CSRF-Token header is present", async () => {
		getCSRFToken.mockResolvedValue("expected-token");

		const res = await POST(makeEvent({ action: "create_snapshot" }));

		expect(res.status).toBe(403);
		expect(createSovereignSnapshot).not.toHaveBeenCalled();
	});

	it("rejects the request with 403 when X-CSRF-Token does not match the session token", async () => {
		getCSRFToken.mockResolvedValue("expected-token");

		const res = await POST(
			makeEvent({ action: "create_snapshot" }, "wrong-token"),
		);

		expect(res.status).toBe(403);
		expect(createSovereignSnapshot).not.toHaveBeenCalled();
	});

	it("proceeds when X-CSRF-Token matches the session token", async () => {
		getCSRFToken.mockResolvedValue("expected-token");

		const res = await POST(
			makeEvent({ action: "create_snapshot" }, "expected-token"),
		);

		expect(res.status).toBe(200);
		expect(createSovereignSnapshot).toHaveBeenCalledTimes(1);
	});
});

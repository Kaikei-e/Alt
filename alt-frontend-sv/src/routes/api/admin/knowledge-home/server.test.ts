import { beforeEach, describe, expect, it, vi } from "vitest";

const { triggerKnowledgeHomeBackfill } = vi.hoisted(() => ({
	triggerKnowledgeHomeBackfill: vi.fn(),
}));

// Real verifyCsrfToken runs so this is an end-to-end test of the route's
// double-submit-cookie guard, not just a mocked comparison.
vi.mock("$lib/api", async (importOriginal) => {
	const actual = await importOriginal<typeof import("$lib/api")>();
	return { ...actual };
});
vi.mock("$lib/server/knowledge-home-admin", () => ({
	fetchKnowledgeHomeAdminSnapshot: vi.fn(),
	pauseKnowledgeHomeBackfill: vi.fn(),
	resumeKnowledgeHomeBackfill: vi.fn(),
	triggerKnowledgeHomeBackfill,
	emitKnowledgeHomeArticleUrlBackfill: vi.fn(),
	startKnowledgeHomeReproject: vi.fn(),
	compareKnowledgeHomeReproject: vi.fn(),
	swapKnowledgeHomeReproject: vi.fn(),
	rollbackKnowledgeHomeReproject: vi.fn(),
	runKnowledgeHomeAudit: vi.fn(),
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
		request: new Request("http://localhost/api/admin/knowledge-home", {
			method: "POST",
			headers,
			body: JSON.stringify(body),
		}),
		locals: {
			user: { traits: { role: "admin" } },
			backendToken: "backend-token",
		},
		cookies: {
			get: (name: string) => (name === "csrf_token" ? cookieCsrf : undefined),
		},
	} as unknown as Parameters<typeof POST>[0];
}

describe("POST /api/admin/knowledge-home — CSRF", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		triggerKnowledgeHomeBackfill.mockResolvedValue({ id: "job-1" });
	});

	it("rejects the request with 403 when no X-CSRF-Token header is present", async () => {
		const res = await POST(
			makeEvent({ action: "trigger", projectionVersion: 1 }),
		);

		expect(res.status).toBe(403);
		expect(triggerKnowledgeHomeBackfill).not.toHaveBeenCalled();
	});

	it("rejects the request with 403 when X-CSRF-Token does not match the double-submit cookie", async () => {
		const res = await POST(
			makeEvent(
				{ action: "trigger", projectionVersion: 1 },
				{ csrfHeader: "wrong-token" },
			),
		);

		expect(res.status).toBe(403);
		expect(triggerKnowledgeHomeBackfill).not.toHaveBeenCalled();
	});

	it("rejects the request with 403 when no csrf cookie was ever issued", async () => {
		const res = await POST(
			makeEvent(
				{ action: "trigger", projectionVersion: 1 },
				{ csrfHeader: "expected-token", cookieCsrf: undefined },
			),
		);

		expect(res.status).toBe(403);
		expect(triggerKnowledgeHomeBackfill).not.toHaveBeenCalled();
	});

	it("proceeds when X-CSRF-Token matches the double-submit cookie", async () => {
		const res = await POST(
			makeEvent(
				{ action: "trigger", projectionVersion: 1 },
				{ csrfHeader: "expected-token" },
			),
		);

		expect(res.status).toBe(200);
		expect(triggerKnowledgeHomeBackfill).toHaveBeenCalledTimes(1);
	});
});

import { beforeEach, describe, expect, it, vi } from "vitest";

const { getCSRFToken, triggerKnowledgeHomeBackfill } = vi.hoisted(() => ({
	getCSRFToken: vi.fn(),
	triggerKnowledgeHomeBackfill: vi.fn(),
}));

vi.mock("$lib/api", () => ({ getCSRFToken }));
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

function makeEvent(body: unknown, csrfHeader?: string) {
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
	} as unknown as Parameters<typeof POST>[0];
}

describe("POST /api/admin/knowledge-home — CSRF", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		triggerKnowledgeHomeBackfill.mockResolvedValue({ id: "job-1" });
	});

	it("rejects the request with 403 when no X-CSRF-Token header is present", async () => {
		getCSRFToken.mockResolvedValue("expected-token");

		const res = await POST(
			makeEvent({ action: "trigger", projectionVersion: 1 }),
		);

		expect(res.status).toBe(403);
		expect(triggerKnowledgeHomeBackfill).not.toHaveBeenCalled();
	});

	it("rejects the request with 403 when X-CSRF-Token does not match the session token", async () => {
		getCSRFToken.mockResolvedValue("expected-token");

		const res = await POST(
			makeEvent({ action: "trigger", projectionVersion: 1 }, "wrong-token"),
		);

		expect(res.status).toBe(403);
		expect(triggerKnowledgeHomeBackfill).not.toHaveBeenCalled();
	});

	it("proceeds when X-CSRF-Token matches the session token", async () => {
		getCSRFToken.mockResolvedValue("expected-token");

		const res = await POST(
			makeEvent(
				{ action: "trigger", projectionVersion: 1 },
				"expected-token",
			),
		);

		expect(res.status).toBe(200);
		expect(triggerKnowledgeHomeBackfill).toHaveBeenCalledTimes(1);
	});
});

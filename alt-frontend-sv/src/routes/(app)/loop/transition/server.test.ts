import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$env/dynamic/private", () => ({
	env: { BACKEND_CONNECT_URL: "http://test-backend:9101" },
}));

vi.mock("$lib/server/knowledge-loop-api", () => ({
	transitionKnowledgeLoopForUser: vi.fn(),
}));

import { transitionKnowledgeLoopForUser } from "$lib/server/knowledge-loop-api";
import { POST } from "./+server";

type Locals = { backendToken?: string };

async function invoke({
	body,
	locals = { backendToken: "test-token" },
}: {
	body: unknown;
	locals?: Locals;
}) {
	const request = new Request("http://localhost/loop/transition", {
		method: "POST",
		headers: { "content-type": "application/json" },
		body: typeof body === "string" ? body : JSON.stringify(body),
	});
	// biome-ignore lint/suspicious/noExplicitAny: SvelteKit RequestEvent mock
	return POST({ request, locals } as any);
}

describe("/loop/transition +server.ts", () => {
	beforeEach(() => {
		vi.mocked(transitionKnowledgeLoopForUser).mockReset();
	});

	it("401 when backendToken is missing", async () => {
		const res = await invoke({ body: {}, locals: {} });
		expect(res.status).toBe(401);
		expect(transitionKnowledgeLoopForUser).not.toHaveBeenCalled();
	});

	it("400 when body fails schema (bad UUID)", async () => {
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "not-a-uuid",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "orient",
				trigger: "user_tap",
				observedProjectionRevision: 1,
			},
		});
		expect(res.status).toBe(400);
		expect(transitionKnowledgeLoopForUser).not.toHaveBeenCalled();
	});

	it("400 when body fails schema (forbidden transition)", async () => {
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "0193c8e5-7d6c-7c4a-b000-000000000001",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "act",
				trigger: "user_tap",
				observedProjectionRevision: 1,
			},
		});
		expect(res.status).toBe(400);
		expect(transitionKnowledgeLoopForUser).not.toHaveBeenCalled();
	});

	it("200 with accepted=true on happy path", async () => {
		vi.mocked(transitionKnowledgeLoopForUser).mockResolvedValue({
			accepted: true,
		});
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "0193c8e5-7d6c-7c4a-b000-000000000002",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "orient",
				trigger: "user_tap",
				observedProjectionRevision: 1,
			},
		});
		expect(res.status).toBe(200);
		const json = await res.json();
		expect(json).toEqual({ accepted: true });
	});

	it("200 silent replay on AlreadyExists (sovereign idempotency)", async () => {
		vi.mocked(transitionKnowledgeLoopForUser).mockRejectedValue(
			Object.assign(new Error("already_exists"), { code: "already_exists" }),
		);
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "0193c8e5-7d6c-7c4a-b000-000000000003",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "orient",
				trigger: "user_tap",
				observedProjectionRevision: 1,
			},
		});
		expect(res.status).toBe(200);
		const json = await res.json();
		expect(json).toEqual({ accepted: true, replay: true });
	});

	it("409 on FailedPrecondition (stale projection revision)", async () => {
		vi.mocked(transitionKnowledgeLoopForUser).mockRejectedValue(
			Object.assign(new Error("projection_stale"), {
				code: "failed_precondition",
			}),
		);
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "0193c8e5-7d6c-7c4a-b000-000000000004",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "orient",
				trigger: "user_tap",
				observedProjectionRevision: 99,
			},
		});
		expect(res.status).toBe(409);
	});

	it("502 on unknown upstream failure", async () => {
		vi.mocked(transitionKnowledgeLoopForUser).mockRejectedValue(
			new Error("network unreachable"),
		);
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "0193c8e5-7d6c-7c4a-b000-000000000005",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "orient",
				trigger: "user_tap",
				observedProjectionRevision: 1,
			},
		});
		expect(res.status).toBe(502);
	});
});

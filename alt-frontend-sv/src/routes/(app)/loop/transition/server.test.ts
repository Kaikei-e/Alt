import { Code, ConnectError } from "@connectrpc/connect";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

	afterEach(() => {
		vi.restoreAllMocks();
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

	it("502 upstream_unreachable on bare Error (fetch TypeError / no ConnectError code)", async () => {
		vi.spyOn(console, "error").mockImplementation(() => {});
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
		const body = await res.json();
		expect(body).toEqual({ error: "upstream_unreachable" });
	});

	it.each([
		["internal", 500, "upstream_internal"],
		["unavailable", 502, "upstream_unavailable"],
		["deadline_exceeded", 504, "timeout"],
		["resource_exhausted", 429, "rate_limited"],
	] as const)("maps Connect-RPC code %s → HTTP %i (%s)", async (code, expectedStatus, expectedError) => {
		vi.mocked(transitionKnowledgeLoopForUser).mockRejectedValue(
			Object.assign(new Error(code), { code }),
		);
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "0193c8e5-7d6c-7c4a-b000-000000000010",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "orient",
				trigger: "user_tap",
				observedProjectionRevision: 1,
			},
		});
		expect(res.status).toBe(expectedStatus);
		const body = await res.json();
		expect(body).toEqual({ error: expectedError });
	});

	it("401 unauthorized on ConnectError code=unauthenticated", async () => {
		vi.mocked(transitionKnowledgeLoopForUser).mockRejectedValue(
			Object.assign(new Error("unauthenticated"), { code: "unauthenticated" }),
		);
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "0193c8e5-7d6c-7c4a-b000-000000000011",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "orient",
				trigger: "user_tap",
				observedProjectionRevision: 1,
			},
		});
		expect(res.status).toBe(401);
		const body = await res.json();
		expect(body).toEqual({ error: "unauthorized" });
	});

	it("400 invalid_argument on ConnectError code=invalid_argument", async () => {
		vi.mocked(transitionKnowledgeLoopForUser).mockRejectedValue(
			Object.assign(new Error("invalid_argument"), {
				code: "invalid_argument",
			}),
		);
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "0193c8e5-7d6c-7c4a-b000-000000000012",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "orient",
				trigger: "user_tap",
				observedProjectionRevision: 1,
			},
		});
		expect(res.status).toBe(400);
		const body = await res.json();
		expect(body).toEqual({ error: "invalid_argument" });
	});

	// Regression (2026-04-24): the previous extractConnectCode only accepted
	// string `code`, but @connectrpc/connect v2 ConnectError.code is numeric.
	// Before the fix, a real ConnectError(Code.InvalidArgument) thrown from
	// the RPC client fell through to the default branch and surfaced as
	// HTTP 502 upstream_unreachable, masking the real 400 response from
	// alt-backend. This test uses the real ConnectError shape.
	it("400 invalid_argument on real ConnectError(Code.InvalidArgument)", async () => {
		vi.mocked(transitionKnowledgeLoopForUser).mockRejectedValue(
			new ConnectError("invalid argument", Code.InvalidArgument),
		);
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "0193c8e5-7d6c-7c4a-b000-000000000013",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "orient",
				trigger: "dwell",
				observedProjectionRevision: 1,
			},
		});
		expect(res.status).toBe(400);
		const body = await res.json();
		expect(body).toEqual({ error: "invalid_argument" });
	});

	it("502 upstream_unavailable on real ConnectError(Code.Unavailable)", async () => {
		vi.mocked(transitionKnowledgeLoopForUser).mockRejectedValue(
			new ConnectError("upstream", Code.Unavailable),
		);
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "0193c8e5-7d6c-7c4a-b000-000000000014",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "orient",
				trigger: "user_tap",
				observedProjectionRevision: 1,
			},
		});
		expect(res.status).toBe(502);
		const body = await res.json();
		expect(body).toEqual({ error: "upstream_unavailable" });
	});

	it("logs loop.transition.unknown_error once for bare TypeError (no Connect code)", async () => {
		const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
		const cause = Object.assign(new Error("ECONNREFUSED"), {
			code: "ECONNREFUSED",
		});
		const err = new TypeError("fetch failed");
		(err as Error & { cause?: unknown }).cause = cause;
		vi.mocked(transitionKnowledgeLoopForUser).mockRejectedValue(err);

		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "0193c8e5-7d6c-7c4a-b000-000000000020",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "orient",
				trigger: "user_tap",
				observedProjectionRevision: 1,
			},
		});

		expect(res.status).toBe(502);
		expect(errSpy).toHaveBeenCalledTimes(1);
		const [tag, payload] = errSpy.mock.calls[0] as [
			string,
			Record<string, unknown>,
		];
		expect(tag).toBe("loop.transition.unknown_error");
		expect(payload).toMatchObject({
			name: "TypeError",
			message: "fetch failed",
			code: null,
		});
		expect(typeof payload.cause).toBe("string");
		expect(payload.cause).toContain("ECONNREFUSED");
	});

	it.each([
		["canceled", 499, "canceled"],
		["unimplemented", 501, "unimplemented"],
		["not_found", 404, "not_found"],
	] as const)("maps specific Connect code %s → HTTP %i (%s) without noisy log", async (code, expectedStatus, expectedError) => {
		const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
		vi.mocked(transitionKnowledgeLoopForUser).mockRejectedValue(
			Object.assign(new Error(code), { code }),
		);

		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "0193c8e5-7d6c-7c4a-b000-000000000021",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "orient",
				trigger: "user_tap",
				observedProjectionRevision: 1,
			},
		});

		expect(res.status).toBe(expectedStatus);
		const body = await res.json();
		expect(body).toEqual({ error: expectedError });
		// Known semantic codes: no diagnostic log spam.
		expect(errSpy).not.toHaveBeenCalled();
	});

	it.each([
		["aborted", 500, "upstream_internal"],
		["out_of_range", 500, "upstream_internal"],
		["data_loss", 500, "upstream_internal"],
		["unknown", 500, "upstream_internal"],
	] as const)("maps catch-all Connect code %s → HTTP %i (%s) and logs once for ops", async (code, expectedStatus, expectedError) => {
		const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
		vi.mocked(transitionKnowledgeLoopForUser).mockRejectedValue(
			Object.assign(new Error(code), { code }),
		);

		const res = await invoke({
			body: {
				lensModeId: "default",
				clientTransitionId: "0193c8e5-7d6c-7c4a-b000-000000000022",
				entryKey: "article:42",
				fromStage: "observe",
				toStage: "orient",
				trigger: "user_tap",
				observedProjectionRevision: 1,
			},
		});

		expect(res.status).toBe(expectedStatus);
		const body = await res.json();
		expect(body).toEqual({ error: expectedError });
		expect(errSpy).toHaveBeenCalledTimes(1);
		const [tag, payload] = errSpy.mock.calls[0] as [
			string,
			Record<string, unknown>,
		];
		expect(tag).toBe("loop.transition.unknown_error");
		expect(payload).toMatchObject({ name: "Error", message: code, code });
	});
});

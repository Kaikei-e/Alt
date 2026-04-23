import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("$env/dynamic/private", () => ({
	env: { BACKEND_CONNECT_URL: "http://test-backend:9101" },
}));

vi.mock("$lib/server/knowledge-loop-api", () => ({
	createAugurSessionFromLoopEntryForUser: vi.fn(),
}));

import { createAugurSessionFromLoopEntryForUser } from "$lib/server/knowledge-loop-api";
import { POST } from "./+server";

type Locals = { backendToken?: string };

async function invoke({
	body,
	locals = { backendToken: "test-token" },
}: {
	body: unknown;
	locals?: Locals;
}) {
	const request = new Request("http://localhost/loop/ask", {
		method: "POST",
		headers: { "content-type": "application/json" },
		body: typeof body === "string" ? body : JSON.stringify(body),
	});
	// biome-ignore lint/suspicious/noExplicitAny: SvelteKit RequestEvent mock
	return POST({ request, locals } as any);
}

describe("/loop/ask +server.ts", () => {
	beforeEach(() => {
		vi.mocked(createAugurSessionFromLoopEntryForUser).mockReset();
	});

	it("401 when backendToken is missing", async () => {
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientHandshakeId: "0193c8e5-7d6c-7c4a-b000-000000000001",
				entryKey: "entry-1",
			},
			locals: {},
		});
		expect(res.status).toBe(401);
		expect(createAugurSessionFromLoopEntryForUser).not.toHaveBeenCalled();
	});

	it("400 when clientHandshakeId is not UUIDv7", async () => {
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientHandshakeId: "not-a-uuid",
				entryKey: "entry-1",
			},
		});
		expect(res.status).toBe(400);
		expect(createAugurSessionFromLoopEntryForUser).not.toHaveBeenCalled();
	});

	it("400 when entryKey is missing", async () => {
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientHandshakeId: "0193c8e5-7d6c-7c4a-b000-000000000002",
			},
		});
		expect(res.status).toBe(400);
		expect(createAugurSessionFromLoopEntryForUser).not.toHaveBeenCalled();
	});

	it("200 with conversationId on happy path", async () => {
		vi.mocked(createAugurSessionFromLoopEntryForUser).mockResolvedValue({
			conversationId: "conv-42",
		});
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientHandshakeId: "0193c8e5-7d6c-7c4a-b000-000000000003",
				entryKey: "entry-1",
			},
		});
		expect(res.status).toBe(200);
		const json = await res.json();
		expect(json).toEqual({ conversationId: "conv-42" });
	});

	it("404 when the loop entry cannot be found in the caller's foreground", async () => {
		vi.mocked(createAugurSessionFromLoopEntryForUser).mockRejectedValue(
			Object.assign(new Error("entry_not_found"), { code: "not_found" }),
		);
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientHandshakeId: "0193c8e5-7d6c-7c4a-b000-000000000004",
				entryKey: "missing",
			},
		});
		expect(res.status).toBe(404);
	});

	it("502 on upstream failure", async () => {
		vi.mocked(createAugurSessionFromLoopEntryForUser).mockRejectedValue(
			new Error("network"),
		);
		const res = await invoke({
			body: {
				lensModeId: "default",
				clientHandshakeId: "0193c8e5-7d6c-7c4a-b000-000000000005",
				entryKey: "entry-1",
			},
		});
		expect(res.status).toBe(502);
	});
});

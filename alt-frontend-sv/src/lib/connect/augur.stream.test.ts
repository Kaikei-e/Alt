import { describe, it, expect, vi, beforeEach } from "vitest";

const streamChat = vi.fn();

vi.mock("@connectrpc/connect", () => ({
	createClient: () => ({ streamChat }),
}));

import { streamAugurChat } from "./augur";

/** Feeds a fixed event list through the stream the client iterates. */
function emits(events: unknown[]) {
	streamChat.mockImplementation(async function* () {
		for (const event of events) {
			yield event;
		}
	});
}

/** Waits for the client's background async loop to drain. */
const settle = () => new Promise((resolve) => setTimeout(resolve, 0));

const transport = {} as never;
const options = { messages: [{ role: "user" as const, content: "why" }] };

describe("streamAugurChat terminal events", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("reports a fallback even when the server sends an empty code", async () => {
		// fallback and error are terminal: the usecase returns right after
		// emitting one, so no `done` follows to rescue the turn. Gating the
		// callback on a truthy payload therefore does not "skip an empty
		// notice" — it strands the caller with no callback at all, and the UI
		// spins forever on a request that already finished.
		emits([{ kind: "fallback", payload: { case: "fallbackCode", value: "" } }]);

		const onFallback = vi.fn();
		streamAugurChat(
			transport,
			options,
			undefined,
			undefined,
			undefined,
			undefined,
			onFallback,
		);
		await settle();

		expect(onFallback).toHaveBeenCalledTimes(1);
	});

	it("reports an error even when the server sends an empty message", async () => {
		emits([{ kind: "error", payload: { case: "errorMessage", value: "" } }]);

		const onError = vi.fn();
		streamAugurChat(
			transport,
			options,
			undefined,
			undefined,
			undefined,
			undefined,
			undefined,
			onError,
		);
		await settle();

		expect(onError).toHaveBeenCalledTimes(1);
		expect(onError.mock.calls[0]?.[0]).toBeInstanceOf(Error);
		expect((onError.mock.calls[0]?.[0] as Error).message).not.toBe("");
	});

	it("still passes a non-empty fallback code through unchanged", async () => {
		emits([
			{
				kind: "fallback",
				payload: { case: "fallbackCode", value: "no context returned" },
			},
		]);

		const onFallback = vi.fn();
		streamAugurChat(
			transport,
			options,
			undefined,
			undefined,
			undefined,
			undefined,
			onFallback,
		);
		await settle();

		expect(onFallback).toHaveBeenCalledWith("no context returned");
	});
});

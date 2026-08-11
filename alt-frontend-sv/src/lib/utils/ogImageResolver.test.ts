import { describe, expect, it, vi } from "vitest";
import { createOgImageResolver } from "./ogImageResolver";

const flush = () => new Promise((r) => setTimeout(r, 30));

describe("createOgImageResolver", () => {
	it("coalesces feeds revealed together into one request", async () => {
		const send = vi.fn().mockResolvedValue(
			new Map([
				["a", "/proxy/a"],
				["b", "/proxy/b"],
			]),
		);
		const resolver = createOgImageResolver({ send, flushMs: 5 });

		const [a, b] = await Promise.all([
			resolver.resolve("a"),
			resolver.resolve("b"),
		]);

		expect(send).toHaveBeenCalledTimes(1);
		expect(send).toHaveBeenCalledWith(["a", "b"]);
		expect(a).toBe("/proxy/a");
		expect(b).toBe("/proxy/b");
	});

	it("asks for a feed only once, however often it scrolls past", async () => {
		const send = vi.fn().mockResolvedValue(new Map([["a", "/proxy/a"]]));
		const resolver = createOgImageResolver({ send, flushMs: 5 });

		expect(await resolver.resolve("a")).toBe("/proxy/a");
		await flush();
		expect(await resolver.resolve("a")).toBe("/proxy/a");
		await flush();

		expect(send).toHaveBeenCalledTimes(1);
	});

	it("remembers a feed the server could not resolve and never re-asks", async () => {
		// An empty map means "asked, and got nothing back" — the server has
		// already recorded the refusal, so asking again would only cost the
		// origin another request.
		const send = vi.fn().mockResolvedValue(new Map());
		const resolver = createOgImageResolver({ send, flushMs: 5 });

		expect(await resolver.resolve("a")).toBeNull();
		await flush();
		expect(await resolver.resolve("a")).toBeNull();
		await flush();

		expect(send).toHaveBeenCalledTimes(1);
	});

	it("caps a batch so one viewport change cannot contact everything at once", async () => {
		const send = vi.fn().mockResolvedValue(new Map());
		const resolver = createOgImageResolver({ send, flushMs: 5, maxBatch: 3 });

		await Promise.all(
			["a", "b", "c", "d", "e"].map((id) => resolver.resolve(id)),
		);

		expect(send).toHaveBeenCalledTimes(2);
		const batches = send.mock.calls.map((call) => (call[0] as string[]).length);
		expect(batches).toEqual([3, 2]);
	});

	it("resolves to null on a transport failure and allows a later retry", async () => {
		// A failure on our side is not the origin refusing, so it must not
		// become a permanent local "no".
		const send = vi
			.fn()
			.mockRejectedValueOnce(new Error("network"))
			.mockResolvedValueOnce(new Map([["a", "/proxy/a"]]));
		const resolver = createOgImageResolver({ send, flushMs: 5 });

		expect(await resolver.resolve("a")).toBeNull();
		await flush();
		expect(await resolver.resolve("a")).toBe("/proxy/a");

		expect(send).toHaveBeenCalledTimes(2);
	});
});

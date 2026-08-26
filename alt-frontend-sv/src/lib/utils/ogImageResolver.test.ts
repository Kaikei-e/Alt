import { Code, ConnectError } from "@connectrpc/connect";
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
		expect(a).toEqual({ status: "resolved", url: "/proxy/a" });
		expect(b).toEqual({ status: "resolved", url: "/proxy/b" });
	});

	it("asks for a feed only once, however often it scrolls past", async () => {
		const send = vi.fn().mockResolvedValue(new Map([["a", "/proxy/a"]]));
		const resolver = createOgImageResolver({ send, flushMs: 5 });

		expect(await resolver.resolve("a")).toEqual({
			status: "resolved",
			url: "/proxy/a",
		});
		await flush();
		expect(await resolver.resolve("a")).toEqual({
			status: "resolved",
			url: "/proxy/a",
		});
		await flush();

		expect(send).toHaveBeenCalledTimes(1);
	});

	it("remembers a feed the server could not resolve and never re-asks", async () => {
		// An empty map means "asked, and got nothing back" — the server has
		// already recorded the refusal, so asking again would only cost the
		// origin another request.
		const send = vi.fn().mockResolvedValue(new Map());
		const resolver = createOgImageResolver({ send, flushMs: 5 });

		expect(await resolver.resolve("a")).toEqual({ status: "absent" });
		await flush();
		expect(await resolver.resolve("a")).toEqual({ status: "absent" });
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

	it("reports a transport failure as unavailable rather than absent", async () => {
		// The distinction the caller renders: "absent" is the origin's answer
		// and is final, "unavailable" is our own failure to ask and must not
		// blank the card for the rest of the session.
		const send = vi.fn().mockRejectedValue(new Error("network"));
		const resolver = createOgImageResolver({ send, flushMs: 5 });

		expect(await resolver.resolve("a")).toEqual({
			status: "unavailable",
			retryAfterMs: null,
		});
	});

	it("does not memoise an unavailable answer, so a later ask can succeed", async () => {
		const send = vi
			.fn()
			.mockRejectedValueOnce(new Error("network"))
			.mockResolvedValueOnce(new Map([["a", "/proxy/a"]]));
		const resolver = createOgImageResolver({ send, flushMs: 5 });

		expect((await resolver.resolve("a")).status).toBe("unavailable");
		await flush();
		expect(await resolver.resolve("a")).toEqual({
			status: "resolved",
			url: "/proxy/a",
		});

		expect(send).toHaveBeenCalledTimes(2);
	});

	it("carries the server's Retry-After through so the caller can honour it", async () => {
		const err = new ConnectError("slot", Code.ResourceExhausted, {
			"retry-after": "8",
		});
		const send = vi.fn().mockRejectedValue(err);
		const resolver = createOgImageResolver({ send, flushMs: 5 });

		expect(await resolver.resolve("a")).toEqual({
			status: "unavailable",
			retryAfterMs: 8000,
		});
	});

	it("treats a deadline as unavailable — the server is still resolving", async () => {
		// The RPC fetches the publisher's page inline behind a per-host slot, so
		// a deadline is the common way a *successful* resolution is missed. The
		// image lands in the store anyway, and the next ask reads it from there
		// without contacting the origin at all.
		const send = vi
			.fn()
			.mockRejectedValue(new ConnectError("deadline", Code.DeadlineExceeded));
		const resolver = createOgImageResolver({ send, flushMs: 5 });

		expect((await resolver.resolve("a")).status).toBe("unavailable");
	});

	it("treats a refusal we cannot argue with as absent, and remembers it", async () => {
		// Unimplemented is the handler saying the image proxy is switched off.
		// Retrying that is a request that can never succeed.
		const send = vi
			.fn()
			.mockRejectedValue(new ConnectError("disabled", Code.Unimplemented));
		const resolver = createOgImageResolver({ send, flushMs: 5 });

		expect(await resolver.resolve("a")).toEqual({ status: "absent" });
		await flush();
		expect(await resolver.resolve("a")).toEqual({ status: "absent" });

		expect(send).toHaveBeenCalledTimes(1);
	});

	it("does not retry a rate limit the publisher itself issued", async () => {
		// ADR-000963 has alt-backend stamp the scope only on failures it has
		// attributed to the third-party site. Re-sending into that is the storm
		// ADR-000884 exists to prevent, so it settles as absent instead.
		const err = new ConnectError("429", Code.ResourceExhausted, {
			"X-Alt-Failure-Scope": "host",
		});
		const send = vi.fn().mockRejectedValue(err);
		const resolver = createOgImageResolver({ send, flushMs: 5 });

		expect(await resolver.resolve("a")).toEqual({ status: "absent" });
	});
});

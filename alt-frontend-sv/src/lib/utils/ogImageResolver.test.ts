import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it, vi } from "vitest";
import { createOgImageResolver } from "./ogImageResolver";
import { OG_RETRY_CEILING_MS } from "./ogImageRetry";

const flush = () => new Promise((r) => setTimeout(r, 30));

/** One server answer, in the two-list shape `resolveOgImages` returns. */
const answer = (
	resolved: Record<string, string> = {},
	unresolved: Record<string, number> = {},
) => ({
	resolved: new Map(Object.entries(resolved)),
	unresolved: new Map(Object.entries(unresolved)),
});

describe("createOgImageResolver", () => {
	it("coalesces feeds revealed together into one request", async () => {
		const send = vi
			.fn()
			.mockResolvedValue(answer({ a: "/proxy/a", b: "/proxy/b" }));
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
		const send = vi.fn().mockResolvedValue(answer({ a: "/proxy/a" }));
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

	it("caps a batch so one viewport change cannot contact everything at once", async () => {
		const send = vi.fn().mockResolvedValue(answer());
		const resolver = createOgImageResolver({ send, flushMs: 5, maxBatch: 3 });

		await Promise.all(
			["a", "b", "c", "d", "e"].map((id) => resolver.resolve(id)),
		);

		expect(send).toHaveBeenCalledTimes(2);
		const batches = send.mock.calls.map((call) => (call[0] as string[]).length);
		expect(batches).toEqual([3, 2]);
	});

	describe("the four answers the server can give about one feed", () => {
		it("takes a feed in `images` as resolved and remembers it", async () => {
			const send = vi.fn().mockResolvedValue(answer({ a: "/proxy/a" }));
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

			expect(send).toHaveBeenCalledTimes(1);
		});

		it("takes retry_after == 0 as the origin's settled no, and never re-asks", async () => {
			// A robots.txt disallow, or a page with no og:image tag. The server
			// has recorded it for the retention window; asking again buys
			// nothing and costs the publisher a request.
			const send = vi.fn().mockResolvedValue(answer({}, { a: 0 }));
			const resolver = createOgImageResolver({ send, flushMs: 5 });

			expect(await resolver.resolve("a")).toEqual({ status: "absent" });
			await flush();
			expect(await resolver.resolve("a")).toEqual({ status: "absent" });
			await flush();

			expect(send).toHaveBeenCalledTimes(1);
		});

		it("carries a retry_after inside the ceiling through as unavailable, unremembered", async () => {
			const send = vi
				.fn()
				.mockResolvedValueOnce(answer({}, { a: 5_000 }))
				.mockResolvedValueOnce(answer({ a: "/proxy/a" }));
			const resolver = createOgImageResolver({ send, flushMs: 5 });

			expect(await resolver.resolve("a")).toEqual({
				status: "unavailable",
				retryAfterMs: 5_000,
			});
			await flush();

			// Not remembered — the whole point of the bar is that the question
			// may be put again once it lifts.
			expect(await resolver.resolve("a")).toEqual({
				status: "resolved",
				url: "/proxy/a",
			});
			expect(send).toHaveBeenCalledTimes(2);
		});

		it("honours a retry_after exactly on the ceiling rather than rounding it away", async () => {
			const send = vi
				.fn()
				.mockResolvedValue(answer({}, { a: OG_RETRY_CEILING_MS }));
			const resolver = createOgImageResolver({ send, flushMs: 5 });

			expect(await resolver.resolve("a")).toEqual({
				status: "unavailable",
				retryAfterMs: OG_RETRY_CEILING_MS,
			});
		});

		it("drops a retry_after above the ceiling to absent, and never re-asks", async () => {
			// A bar longer than a card can be held open for. The tile takes its
			// fallback now rather than shimmering at a wait it cannot honour.
			const send = vi
				.fn()
				.mockResolvedValue(answer({}, { a: OG_RETRY_CEILING_MS + 1 }));
			const resolver = createOgImageResolver({ send, flushMs: 5 });

			expect(await resolver.resolve("a")).toEqual({ status: "absent" });
			await flush();
			expect(await resolver.resolve("a")).toEqual({ status: "absent" });
			await flush();

			expect(send).toHaveBeenCalledTimes(1);
		});

		it("takes a feed in neither list as unavailable, and does not remember it", async () => {
			// The batch cap trimmed it, no row exists, or its page URL was
			// unusable. No origin request was spent on it, so re-asking costs
			// only us.
			const send = vi
				.fn()
				.mockResolvedValueOnce(answer({ b: "/proxy/b" }))
				.mockResolvedValueOnce(answer({ a: "/proxy/a" }));
			const resolver = createOgImageResolver({ send, flushMs: 5 });

			const [a] = await Promise.all([
				resolver.resolve("a"),
				resolver.resolve("b"),
			]);
			expect(a).toEqual({ status: "unavailable", retryAfterMs: null });
			await flush();

			expect(await resolver.resolve("a")).toEqual({
				status: "resolved",
				url: "/proxy/a",
			});
			expect(send).toHaveBeenCalledTimes(2);
		});

		it("prefers a resolution over an unresolved row for the same feed", async () => {
			// The server should never emit both. If it does, the picture it
			// managed to produce is the more useful of the two answers.
			const send = vi
				.fn()
				.mockResolvedValue(answer({ a: "/proxy/a" }, { a: 5_000 }));
			const resolver = createOgImageResolver({ send, flushMs: 5 });

			expect(await resolver.resolve("a")).toEqual({
				status: "resolved",
				url: "/proxy/a",
			});
		});
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
			.mockResolvedValueOnce(answer({ a: "/proxy/a" }));
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

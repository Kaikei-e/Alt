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
	it("dispatches independent per-card requests for feeds revealed together", async () => {
		const send = vi.fn().mockImplementation((ids: string[]) => {
			const id = ids[0]!;
			return Promise.resolve(answer({ [id]: `/proxy/${id}` }));
		});
		const resolver = createOgImageResolver({ send });

		const [a, b] = await Promise.all([
			resolver.resolve("a"),
			resolver.resolve("b"),
		]);

		expect(send).toHaveBeenCalledTimes(2);
		expect(send).toHaveBeenCalledWith(["a"]);
		expect(send).toHaveBeenCalledWith(["b"]);
		expect(a).toEqual({ status: "resolved", url: "/proxy/a" });
		expect(b).toEqual({ status: "resolved", url: "/proxy/b" });
	});

	it("asks for a feed only once, however often it scrolls past", async () => {
		const send = vi.fn().mockResolvedValue(answer({ a: "/proxy/a" }));
		const resolver = createOgImageResolver({ send });

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

	it("bounds concurrent in-flight requests to maxConcurrent", async () => {
		let inFlight = 0;
		let maxObserved = 0;
		const deferreds = new Map<
			string,
			(val: ReturnType<typeof answer>) => void
		>();
		const started = new Map<string, () => void>();
		const whenStarted = (id: string) =>
			new Promise<void>((r) => {
				if (send.mock.calls.some(([ids]) => ids[0] === id)) {
					r();
				} else {
					started.set(id, r);
				}
			});

		const send = vi.fn().mockImplementation((ids: string[]) => {
			inFlight++;
			maxObserved = Math.max(maxObserved, inFlight);
			const id = ids[0]!;
			started.get(id)?.();
			return new Promise<ReturnType<typeof answer>>((resolve) => {
				deferreds.set(id, (val) => {
					inFlight--;
					resolve(val);
				});
			});
		});
		const resolver = createOgImageResolver({ send, maxConcurrent: 2 });

		const promises = ["a", "b", "c", "d", "e"].map((id) =>
			resolver.resolve(id),
		);
		await whenStarted("a");
		await whenStarted("b");

		expect(maxObserved).toBe(2);
		expect(send).toHaveBeenCalledTimes(2);
		expect(send).toHaveBeenCalledWith(["a"]);
		expect(send).toHaveBeenCalledWith(["b"]);

		// Free slot 1 -> "c" starts
		deferreds.get("a")!(answer({ a: "/proxy/a" }));
		await whenStarted("c");

		expect(send).toHaveBeenCalledWith(["c"]);
		expect(maxObserved).toBe(2);

		// Resolve remaining in order
		deferreds.get("b")!(answer({ b: "/proxy/b" }));
		await whenStarted("d");
		deferreds.get("c")!(answer({ c: "/proxy/c" }));
		await whenStarted("e");
		deferreds.get("d")!(answer({ d: "/proxy/d" }));
		deferreds.get("e")!(answer({ e: "/proxy/e" }));

		await Promise.all(promises);
		expect(send).toHaveBeenCalledTimes(5);
	});

	it("settles a fast card independently while a slow card remains pending without batch barrier", async () => {
		let resolveSlow!: (val: ReturnType<typeof answer>) => void;
		const slowDeferred = new Promise<ReturnType<typeof answer>>((r) => {
			resolveSlow = r;
		});

		const send = vi.fn().mockImplementation((ids: string[]) => {
			if (ids.includes("slow")) return slowDeferred;
			return Promise.resolve(answer({ fast: "/proxy/fast.png" }));
		});
		const resolver = createOgImageResolver({ send });

		const slowPromise = resolver.resolve("slow");
		const fastPromise = resolver.resolve("fast");

		// Fast card MUST resolve independently without waiting for slow card
		const fastResult = await fastPromise;
		expect(fastResult).toEqual({ status: "resolved", url: "/proxy/fast.png" });

		// Verify slow card has not settled yet
		let slowSettled = false;
		slowPromise.then(() => {
			slowSettled = true;
		});
		await Promise.resolve();
		expect(slowSettled).toBe(false);

		// Now let slow card finish
		resolveSlow(answer({ slow: "/proxy/slow.png" }));
		const slowResult = await slowPromise;
		expect(slowResult).toEqual({ status: "resolved", url: "/proxy/slow.png" });
	});

	it("begins queued request as soon as ANY slot frees without waiting for group completion", async () => {
		let resolveSlotA!: (val: ReturnType<typeof answer>) => void;
		let resolveSlotB!: (val: ReturnType<typeof answer>) => void;
		const slotADeferred = new Promise<ReturnType<typeof answer>>((r) => {
			resolveSlotA = r;
		});
		const slotBDeferred = new Promise<ReturnType<typeof answer>>((r) => {
			resolveSlotB = r;
		});

		const send = vi.fn().mockImplementation((ids: string[]) => {
			if (ids.includes("a")) return slotADeferred;
			if (ids.includes("b")) return slotBDeferred;
			if (ids.includes("c")) return Promise.resolve(answer({ c: "/proxy/c" }));
			return Promise.resolve(answer());
		});

		// Concurrency cap = 2
		const resolver = createOgImageResolver({ send, maxConcurrent: 2 });

		const pA = resolver.resolve("a");
		const pB = resolver.resolve("b");
		const pC = resolver.resolve("c");

		await Promise.resolve();
		// Slots 1 and 2 are filled with "a" and "b"
		expect(send).toHaveBeenCalledWith(["a"]);
		expect(send).toHaveBeenCalledWith(["b"]);
		expect(send).not.toHaveBeenCalledWith(["c"]);

		// Slot 2 ("b") completes. "c" MUST start immediately while "a" is still pending in slot 1
		resolveSlotB(answer({ b: "/proxy/b" }));
		await pB;
		await Promise.resolve();

		expect(send).toHaveBeenCalledWith(["c"]);
		expect(await pC).toEqual({ status: "resolved", url: "/proxy/c" });

		// Complete slot 1
		resolveSlotA(answer({ a: "/proxy/a" }));
		expect(await pA).toEqual({ status: "resolved", url: "/proxy/a" });
	});

	it("allows retry after a synchronous throw in send without permanently wedging the feed", async () => {
		let shouldThrow = true;
		const send = vi.fn().mockImplementation((ids: string[]) => {
			if (shouldThrow) {
				shouldThrow = false;
				throw new Error("sync throw in send");
			}
			const id = ids[0]!;
			return Promise.resolve(answer({ [id]: `/proxy/${id}` }));
		});
		const resolver = createOgImageResolver({ send });

		const firstAttempt = await resolver.resolve("retry-feed");
		expect(firstAttempt.status).toBe("unavailable");

		// Retry must not be blocked by a stale inFlight promise
		const secondAttempt = await resolver.resolve("retry-feed");
		expect(secondAttempt).toEqual({
			status: "resolved",
			url: "/proxy/retry-feed",
		});
		expect(send).toHaveBeenCalledTimes(2);
	});

	it("normalizes invalid or degenerate maxConcurrent options to safe positive integer", async () => {
		for (const invalid of [NaN, Infinity, -3, 0, undefined]) {
			const send = vi
				.fn()
				.mockImplementation((ids: string[]) =>
					Promise.resolve(answer({ [ids[0]!]: `/proxy/${ids[0]!}` })),
				);
			const resolver = createOgImageResolver({ send, maxConcurrent: invalid });
			const result = await resolver.resolve("test");
			expect(result.status).toBe("resolved");
		}

		// Fractional should truncate to integer floor (e.g. 2.9 -> 2)
		let inFlight = 0;
		let maxObserved = 0;
		const deferreds = new Map<
			string,
			(val: ReturnType<typeof answer>) => void
		>();
		const send = vi.fn().mockImplementation((ids: string[]) => {
			inFlight++;
			maxObserved = Math.max(maxObserved, inFlight);
			return new Promise<ReturnType<typeof answer>>((resolve) => {
				deferreds.set(ids[0]!, resolve);
			});
		});
		const resolver = createOgImageResolver({ send, maxConcurrent: 2.9 });
		const p1 = resolver.resolve("f1");
		const p2 = resolver.resolve("f2");
		const p3 = resolver.resolve("f3");

		await Promise.resolve();
		expect(maxObserved).toBe(2);
		expect(send).toHaveBeenCalledTimes(2);

		deferreds.get("f1")!(answer({ f1: "/proxy/f1" }));
		await p1;
		await Promise.resolve();

		expect(send).toHaveBeenCalledWith(["f3"]);
		deferreds.get("f2")!(answer({ f2: "/proxy/f2" }));
		deferreds.get("f3")!(answer({ f3: "/proxy/f3" }));
		await Promise.all([p2, p3]);
	});

	it("releases concurrency slot when a request fails and continues processing queue", async () => {
		let rejectSlotA!: (err: Error) => void;
		const slotADeferred = new Promise<ReturnType<typeof answer>>(
			(_, reject) => {
				rejectSlotA = reject;
			},
		);
		const send = vi.fn().mockImplementation((ids: string[]) => {
			if (ids.includes("fail")) return slotADeferred;
			if (ids.includes("next"))
				return Promise.resolve(answer({ next: "/proxy/next" }));
			return Promise.resolve(answer());
		});
		const resolver = createOgImageResolver({ send, maxConcurrent: 1 });

		const pFail = resolver.resolve("fail");
		const pNext = resolver.resolve("next");

		await flush();
		expect(send).toHaveBeenCalledWith(["fail"]);
		expect(send).not.toHaveBeenCalledWith(["next"]);

		// Reject "fail"
		rejectSlotA(new Error("network failure"));
		const failResult = await pFail;
		expect(failResult.status).toBe("unavailable");

		// "next" must have been started upon failure release
		await flush();
		expect(send).toHaveBeenCalledWith(["next"]);
		expect(await pNext).toEqual({ status: "resolved", url: "/proxy/next" });
	});

	it("reuses existing in-flight promise for new arrivals of the same feedId while in-flight", async () => {
		let resolvePending!: (val: ReturnType<typeof answer>) => void;
		const pendingDeferred = new Promise<ReturnType<typeof answer>>((r) => {
			resolvePending = r;
		});
		const send = vi.fn().mockImplementation(() => pendingDeferred);
		const resolver = createOgImageResolver({ send });

		const p1 = resolver.resolve("same-feed");
		const p2 = resolver.resolve("same-feed");

		expect(send).toHaveBeenCalledTimes(1);
		expect(send).toHaveBeenCalledWith(["same-feed"]);

		resolvePending(answer({ "same-feed": "/proxy/same" }));
		const [r1, r2] = await Promise.all([p1, p2]);
		expect(r1).toEqual({ status: "resolved", url: "/proxy/same" });
		expect(r2).toEqual({ status: "resolved", url: "/proxy/same" });
	});

	describe("the four answers the server can give about one feed", () => {
		it("takes a feed in `images` as resolved and remembers it", async () => {
			const send = vi.fn().mockResolvedValue(answer({ a: "/proxy/a" }));
			const resolver = createOgImageResolver({ send });

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
			const resolver = createOgImageResolver({ send });

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
			const resolver = createOgImageResolver({ send });

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
			const resolver = createOgImageResolver({ send });

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
			const resolver = createOgImageResolver({ send });

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
			let aAttempts = 0;
			const send = vi.fn().mockImplementation((ids: string[]) => {
				const id = ids[0]!;
				if (id === "a") {
					aAttempts++;
					if (aAttempts === 1) {
						// First attempt for "a": neither list returned
						return Promise.resolve(answer());
					}
					// Second attempt for "a": resolved
					return Promise.resolve(answer({ a: "/proxy/a" }));
				}
				if (id === "b") {
					return Promise.resolve(answer({ b: "/proxy/b" }));
				}
				return Promise.resolve(answer());
			});
			const resolver = createOgImageResolver({ send });

			const [a, b] = await Promise.all([
				resolver.resolve("a"),
				resolver.resolve("b"),
			]);
			expect(a).toEqual({ status: "unavailable", retryAfterMs: null });
			expect(b).toEqual({ status: "resolved", url: "/proxy/b" });
			await flush();

			expect(await resolver.resolve("a")).toEqual({
				status: "resolved",
				url: "/proxy/a",
			});
			expect(send).toHaveBeenCalledTimes(3);
		});

		it("prefers a resolution over an unresolved row for the same feed", async () => {
			// The server should never emit both. If it does, the picture it
			// managed to produce is the more useful of the two answers.
			const send = vi
				.fn()
				.mockResolvedValue(answer({ a: "/proxy/a" }, { a: 5_000 }));
			const resolver = createOgImageResolver({ send });

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
		const resolver = createOgImageResolver({ send });

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
		const resolver = createOgImageResolver({ send });

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
		const resolver = createOgImageResolver({ send });

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
		const resolver = createOgImageResolver({ send });

		expect((await resolver.resolve("a")).status).toBe("unavailable");
	});

	it("treats a refusal we cannot argue with as absent, and remembers it", async () => {
		// Unimplemented is the handler saying the image proxy is switched off.
		// Retrying that is a request that can never succeed.
		const send = vi
			.fn()
			.mockRejectedValue(new ConnectError("disabled", Code.Unimplemented));
		const resolver = createOgImageResolver({ send });

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
		const resolver = createOgImageResolver({ send });

		expect(await resolver.resolve("a")).toEqual({ status: "absent" });
	});
});

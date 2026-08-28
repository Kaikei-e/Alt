import { describe, expect, it, vi } from "vitest";
import { type LoadProxyImageDeps, loadProxyImage } from "./loadProxyImage";

function makeDeps(overrides: Partial<LoadProxyImageDeps> = {}): {
	deps: LoadProxyImageDeps;
	sleeps: number[];
} {
	const sleeps: number[] = [];
	const deps: LoadProxyImageDeps = {
		fetch: vi.fn(async () => new Response(new Blob(), { status: 200 })),
		acquire: vi.fn(async () => () => {}),
		sleep: vi.fn(async (ms: number) => {
			sleeps.push(ms);
		}),
		random: () => 0, // deterministic: no jitter
		...overrides,
	};
	return { deps, sleeps };
}

describe("loadProxyImage", () => {
	it("returns absent without fetching when there is no proxy URL", async () => {
		const { deps } = makeDeps();
		const out = await loadProxyImage(undefined, deps);
		expect(out).toEqual({ status: "absent" });
		expect(deps.fetch).not.toHaveBeenCalled();
	});

	it("reports loaded on a 200, and nothing else", async () => {
		// The verdict is the whole payload: the caller renders the proxy URL it
		// already holds, so there is no URL to hand back here.
		const { deps } = makeDeps();
		const out = await loadProxyImage("/v1/images/proxy/s/abc", deps);
		expect(out).toEqual({ status: "loaded" });
		expect(deps.fetch).toHaveBeenCalledTimes(1);
	});

	it("consumes the response body on success", async () => {
		// Regression guard, do not delete the `res.blob()` this pins. The
		// browser commits a response to its HTTP cache only once the body has
		// been read, and that cache entry is what makes the <img src={proxyUrl}>
		// rendered next a cache hit rather than a second trip to the proxy.
		// Dropping the read as "we never use the blob" doubles the network cost
		// of every thumbnail without failing any other test here.
		const blob = vi.fn(async () => new Blob());
		const res = { ok: true, status: 200, blob } as unknown as Response;
		const { deps } = makeDeps({ fetch: vi.fn(async () => res) });

		const out = await loadProxyImage("/v1/images/proxy/s/abc", deps);

		expect(out).toEqual({ status: "loaded" });
		expect(blob).toHaveBeenCalledTimes(1);
	});

	it("retries a transient 429 and succeeds on the next attempt", async () => {
		// A 429 is the proxy's per-host budget, not the origin's answer. Only a
		// status code can tell the two apart — an <img onerror> sees the same
		// nothing for both — and collapsing this one to the fallback is the
		// regression this loader exists to prevent.
		const fetch = vi
			.fn()
			.mockResolvedValueOnce(new Response("", { status: 429 }))
			.mockResolvedValueOnce(new Response(new Blob(), { status: 200 }));
		const { deps, sleeps } = makeDeps({ fetch });

		const out = await loadProxyImage("/v1/images/proxy/s/abc", deps);

		expect(out).toEqual({ status: "loaded" });
		expect(fetch).toHaveBeenCalledTimes(2);
		expect(fetch).toHaveBeenNthCalledWith(
			2,
			"/v1/images/proxy/s/abc",
			expect.anything(),
		);
		expect(sleeps).toEqual([1500]); // first backoff, no jitter
	});

	it("gives up as absent after exhausting retries on persistent 429", async () => {
		const fetch = vi.fn(async () => new Response("", { status: 429 }));
		const { deps, sleeps } = makeDeps({ fetch });

		const out = await loadProxyImage("/v1/images/proxy/s/abc", deps);

		expect(out).toEqual({ status: "absent" });
		expect(fetch).toHaveBeenCalledTimes(3); // 1 initial + 2 retries
		expect(sleeps).toEqual([1500, 3000]);
	});

	it("treats 403 as permanent and does not retry", async () => {
		// The mirror image of the 429 case: the origin has refused, and paying
		// two more requests and 4.5s of shimmer to be refused again helps
		// nobody. This is the distinction the status probe buys.
		const fetch = vi.fn(async () => new Response("", { status: 403 }));
		const { deps, sleeps } = makeDeps({ fetch });

		const out = await loadProxyImage("/v1/images/proxy/s/abc", deps);

		expect(out).toEqual({ status: "absent" });
		expect(fetch).toHaveBeenCalledTimes(1);
		expect(sleeps).toEqual([]);
	});

	it("retries a network error then resolves", async () => {
		const fetch = vi
			.fn()
			.mockRejectedValueOnce(new Error("network down"))
			.mockResolvedValueOnce(new Response(new Blob(), { status: 200 }));
		const { deps } = makeDeps({ fetch });

		const out = await loadProxyImage("/v1/images/proxy/s/abc", deps);
		expect(out).toEqual({ status: "loaded" });
		expect(fetch).toHaveBeenCalledTimes(2);
	});

	it("acquires and releases a queue slot on every attempt", async () => {
		const release = vi.fn();
		const acquire = vi.fn(async () => release);
		const fetch = vi
			.fn()
			.mockResolvedValueOnce(new Response("", { status: 502 }))
			.mockResolvedValueOnce(new Response(new Blob(), { status: 200 }));
		const { deps } = makeDeps({ acquire, fetch });

		await loadProxyImage("/v1/images/proxy/s/abc", deps);

		expect(acquire).toHaveBeenCalledTimes(2);
		expect(release).toHaveBeenCalledTimes(2); // released even after the 502
	});

	it("stops immediately when the signal is already aborted", async () => {
		const { deps } = makeDeps();
		const ctrl = new AbortController();
		ctrl.abort();

		const out = await loadProxyImage(
			"/v1/images/proxy/s/abc",
			deps,
			ctrl.signal,
		);
		expect(out).toEqual({ status: "absent" });
		expect(deps.fetch).not.toHaveBeenCalled();
	});
});

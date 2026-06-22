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
		createObjectURL: vi.fn(() => "blob:object-url"),
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

	it("returns the object URL on a 200", async () => {
		const { deps } = makeDeps();
		const out = await loadProxyImage("/v1/images/proxy/s/abc", deps);
		expect(out).toEqual({ status: "loaded", objectUrl: "blob:object-url" });
		expect(deps.fetch).toHaveBeenCalledTimes(1);
	});

	it("retries a transient 429 and succeeds on the next attempt", async () => {
		const fetch = vi
			.fn()
			.mockResolvedValueOnce(new Response("", { status: 429 }))
			.mockResolvedValueOnce(new Response(new Blob(), { status: 200 }));
		const { deps, sleeps } = makeDeps({ fetch });

		const out = await loadProxyImage("/v1/images/proxy/s/abc", deps);

		expect(out.status).toBe("loaded");
		expect(fetch).toHaveBeenCalledTimes(2);
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
		const fetch = vi.fn(async () => new Response("", { status: 403 }));
		const { deps } = makeDeps({ fetch });

		const out = await loadProxyImage("/v1/images/proxy/s/abc", deps);

		expect(out).toEqual({ status: "absent" });
		expect(fetch).toHaveBeenCalledTimes(1);
	});

	it("retries a network error then resolves", async () => {
		const fetch = vi
			.fn()
			.mockRejectedValueOnce(new Error("network down"))
			.mockResolvedValueOnce(new Response(new Blob(), { status: 200 }));
		const { deps } = makeDeps({ fetch });

		const out = await loadProxyImage("/v1/images/proxy/s/abc", deps);
		expect(out.status).toBe("loaded");
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

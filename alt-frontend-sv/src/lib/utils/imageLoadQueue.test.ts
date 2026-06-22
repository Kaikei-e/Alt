import { describe, expect, it } from "vitest";
import { extractImageHost, ImageLoadQueue } from "./imageLoadQueue";

const tick = () => new Promise((r) => setTimeout(r, 0));

/** Build a proxy URL whose base64 segment decodes to the given upstream host. */
function proxy(host: string): string {
	return `/v1/images/proxy/sig/${btoa(`https://${host}/x.png`)}`;
}

describe("extractImageHost", () => {
	it("decodes the upstream host from the base64 proxy segment", () => {
		expect(extractImageHost(proxy("res.cloudinary.com"))).toBe(
			"res.cloudinary.com",
		);
		expect(extractImageHost(proxy("img.example.com"))).toBe("img.example.com");
	});

	it("returns null for a non-proxy URL", () => {
		expect(extractImageHost("/not/a/proxy/url")).toBeNull();
	});

	it("returns null when the segment is not decodable to a URL", () => {
		expect(extractImageHost("/v1/images/proxy/sig/@@@notbase64@@@")).toBeNull();
	});
});

describe("ImageLoadQueue", () => {
	it("grants immediately while under the per-host limit", async () => {
		const q = new ImageLoadQueue({ perHost: 2, global: 8 });
		const r1 = await q.acquire(proxy("a.com"));
		const r2 = await q.acquire(proxy("a.com"));
		expect(typeof r1).toBe("function");
		expect(typeof r2).toBe("function");
	});

	it("blocks the 3rd concurrent request to the same host until a slot frees", async () => {
		const q = new ImageLoadQueue({ perHost: 2, global: 8 });
		const r1 = await q.acquire(proxy("a.com"));
		await q.acquire(proxy("a.com"));

		let granted = false;
		const p3 = q.acquire(proxy("a.com")).then((r) => {
			granted = true;
			return r;
		});

		await tick();
		expect(granted).toBe(false); // per-host cap (2) reached

		r1(); // free one slot
		await p3;
		expect(granted).toBe(true);
	});

	it("does not let one saturated host block a different host", async () => {
		const q = new ImageLoadQueue({ perHost: 2, global: 8 });
		await q.acquire(proxy("a.com"));
		await q.acquire(proxy("a.com"));

		let otherGranted = false;
		q.acquire(proxy("b.com")).then(() => {
			otherGranted = true;
		});

		await tick();
		expect(otherGranted).toBe(true); // b.com is independent of a.com's cap
	});

	it("enforces the global ceiling across hosts", async () => {
		const q = new ImageLoadQueue({ perHost: 2, global: 2 });
		const ra = await q.acquire(proxy("a.com"));
		await q.acquire(proxy("b.com"));

		let granted = false;
		const pc = q.acquire(proxy("c.com")).then((r) => {
			granted = true;
			return r;
		});

		await tick();
		expect(granted).toBe(false); // global cap (2) reached

		ra();
		await pc;
		expect(granted).toBe(true);
	});

	it("releases idempotently (double release does not over-credit the budget)", async () => {
		const q = new ImageLoadQueue({ perHost: 1, global: 8 });
		const r1 = await q.acquire(proxy("a.com"));
		r1();
		r1(); // second call must be a no-op

		// Only one same-host slot exists; acquiring two more must still serialize.
		const r2 = await q.acquire(proxy("a.com"));
		let granted = false;
		q.acquire(proxy("a.com")).then(() => {
			granted = true;
		});
		await tick();
		expect(granted).toBe(false);
		r2();
		await tick();
		expect(granted).toBe(true);
	});
});

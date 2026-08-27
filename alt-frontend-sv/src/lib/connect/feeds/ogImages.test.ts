import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@connectrpc/connect", () => ({
	createClient: vi.fn(),
}));

vi.mock("$lib/gen/alt/feeds/v2/feeds_pb", () => ({
	FeedService: {},
}));

import type { Transport } from "@connectrpc/connect";
import { createClient } from "@connectrpc/connect";
import { resolveOgImages } from "./ogImages";

const mockedCreateClient = vi.mocked(createClient);

describe("resolveOgImages", () => {
	let transport: Transport;
	let client: { resolveOgImages: ReturnType<typeof vi.fn> };

	beforeEach(() => {
		transport = {} as Transport;
		client = { resolveOgImages: vi.fn() };
		mockedCreateClient.mockReturnValue(client as never);
	});

	it("keeps the two lists apart so a caller can tell the four answers apart", async () => {
		client.resolveOgImages.mockResolvedValue({
			images: [{ feedId: "a", ogImageProxyUrl: "/proxy/a" }],
			unresolved: [{ feedId: "b", retryAfterSeconds: 0n }],
		});

		const result = await resolveOgImages(transport, ["a", "b", "c"]);

		expect(result.resolved).toEqual(new Map([["a", "/proxy/a"]]));
		expect(result.unresolved).toEqual(new Map([["b", 0]]));
		// "c" is in neither list — the server never reached it, and the caller
		// must be able to see that as its own answer.
		expect(result.unresolved.has("c")).toBe(false);
	});

	it("converts the proto's int64 retry_after from bigint seconds to number milliseconds", async () => {
		// protobuf-es maps int64 to bigint. Leaking that upwards makes every
		// arithmetic comparison against our millisecond constants a TS2365.
		client.resolveOgImages.mockResolvedValue({
			images: [],
			unresolved: [
				{ feedId: "a", retryAfterSeconds: 5n },
				{ feedId: "b", retryAfterSeconds: 86_400n },
			],
		});

		const result = await resolveOgImages(transport, ["a", "b"]);

		expect(result.unresolved.get("a")).toBe(5_000);
		expect(result.unresolved.get("b")).toBe(86_400_000);
		expect(typeof result.unresolved.get("a")).toBe("number");
		expect(typeof result.unresolved.get("b")).toBe("number");
	});

	it("keeps a zero retry_after as a present key, not a missing one", async () => {
		// Zero is the settled "asked and refused" answer. Dropping the entry
		// because it is falsy would turn it into "never asked", which is the
		// one outcome that licences an immediate re-ask.
		client.resolveOgImages.mockResolvedValue({
			images: [],
			unresolved: [{ feedId: "a", retryAfterSeconds: 0n }],
		});

		const result = await resolveOgImages(transport, ["a"]);

		expect(result.unresolved.has("a")).toBe(true);
		expect(result.unresolved.get("a")).toBe(0);
	});

	it("tolerates a response from a server that predates the unresolved field", async () => {
		client.resolveOgImages.mockResolvedValue({
			images: [{ feedId: "a", ogImageProxyUrl: "/proxy/a" }],
		});

		const result = await resolveOgImages(transport, ["a", "b"]);

		expect(result.resolved).toEqual(new Map([["a", "/proxy/a"]]));
		expect(result.unresolved.size).toBe(0);
	});

	it("drops entries with no feed id and resolutions with a blank URL", async () => {
		client.resolveOgImages.mockResolvedValue({
			images: [
				{ feedId: "", ogImageProxyUrl: "/proxy/nobody" },
				{ feedId: "a", ogImageProxyUrl: "" },
			],
			unresolved: [{ feedId: "", retryAfterSeconds: 5n }],
		});

		const result = await resolveOgImages(transport, ["a"]);

		expect(result.resolved.size).toBe(0);
		expect(result.unresolved.size).toBe(0);
	});

	it("clamps a negative retry_after to zero rather than propagating it", async () => {
		client.resolveOgImages.mockResolvedValue({
			images: [],
			unresolved: [{ feedId: "a", retryAfterSeconds: -30n }],
		});

		const result = await resolveOgImages(transport, ["a"]);

		expect(result.unresolved.get("a")).toBe(0);
	});

	it("does not call the RPC for an empty batch", async () => {
		const result = await resolveOgImages(transport, []);

		expect(client.resolveOgImages).not.toHaveBeenCalled();
		expect(result.resolved.size).toBe(0);
		expect(result.unresolved.size).toBe(0);
	});
});

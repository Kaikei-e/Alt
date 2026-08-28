import type { Transport } from "@connectrpc/connect";
import { describe, expect, it } from "vitest";

import { batchPrefetchArticleContent, fetchArticleContent } from "./articles";

interface RecordedCall {
	method: string;
	header: Headers;
	input: Record<string, unknown>;
	timeoutMs: number | undefined;
}

/**
 * Minimal Transport that records what `createClient` handed it and answers
 * with an empty message. Enough to pin the wire shape of a unary call without
 * standing up connect-web, a proxy, or a fetch.
 */
function recordingTransport(calls: RecordedCall[]): Transport {
	// Params are annotated loosely and the whole object is cast once: the real
	// Transport signature is generic over the method's message descriptors, and
	// satisfying it would mean building a real response message per RPC for no
	// gain — the assertions here are about the request half.
	return {
		unary: async (
			method: { name: string; parent: unknown },
			_signal: AbortSignal | undefined,
			timeoutMs: number | undefined,
			header: HeadersInit | undefined,
			input: unknown,
		) => {
			calls.push({
				method: method.name,
				header: new Headers(header),
				input: input as Record<string, unknown>,
				timeoutMs,
			});
			return {
				stream: false as const,
				service: method.parent,
				method,
				header: new Headers(),
				message: {
					url: "",
					content: "",
					articleId: "",
					ogImageUrl: "",
					ogImageProxyUrl: "",
					acceptedCount: 1,
					shedCount: 0,
					rejectedCount: 0,
					skippedSameHostCount: 0,
				},
				trailer: new Headers(),
			};
		},
		stream: () => {
			throw new Error("not used");
		},
	} as unknown as Transport;
}

describe("batchPrefetchArticleContent", () => {
	it("sends the urls to BatchPrefetchArticleContent and returns the receipt", async () => {
		const calls: RecordedCall[] = [];

		const result = await batchPrefetchArticleContent(
			recordingTransport(calls),
			["https://a.example/1", "https://b.example/2"],
		);

		expect(calls).toHaveLength(1);
		expect(calls[0]?.method).toBe("BatchPrefetchArticleContent");
		expect(calls[0]?.input.urls).toEqual([
			"https://a.example/1",
			"https://b.example/2",
		]);
		expect(result.acceptedCount).toBe(1);
	});

	// The warm is nobody's foreground request, so the call site has to be able
	// to say so. This module stays free of `transport-client` (and therefore of
	// `$app/paths`) because the BFF imports it server-side; the header value
	// itself is chosen one layer up, in `$lib/api/client/articles`.
	it("forwards caller-supplied headers", async () => {
		const calls: RecordedCall[] = [];

		await batchPrefetchArticleContent(
			recordingTransport(calls),
			["https://a.example/1"],
			{ "x-alt-fetch-priority": "low" },
		);

		expect(calls[0]?.header.get("x-alt-fetch-priority")).toBe("low");
	});

	// One attempt per URL per call, and the client must not poll. A warm that
	// hangs must not hold a connection slot for the reader's own request.
	it("bounds the call with a short timeout", async () => {
		const calls: RecordedCall[] = [];

		await batchPrefetchArticleContent(recordingTransport(calls), [
			"https://a.example/1",
		]);

		expect(calls[0]?.timeoutMs).toBeGreaterThan(0);
		expect(calls[0]?.timeoutMs).toBeLessThanOrEqual(10_000);
	});
});

describe("fetchArticleContent", () => {
	it("forwards caller-supplied headers, so a call site can set its priority", async () => {
		const calls: RecordedCall[] = [];

		await fetchArticleContent(
			recordingTransport(calls),
			"https://a.example/1",
			undefined,
			undefined,
			{ "x-alt-fetch-priority": "high" },
		);

		expect(calls[0]?.header.get("x-alt-fetch-priority")).toBe("high");
	});

	it("sends nothing extra when no headers are supplied", async () => {
		const calls: RecordedCall[] = [];

		await fetchArticleContent(recordingTransport(calls), "https://a.example/1");

		expect(calls[0]?.header.get("x-alt-fetch-priority")).toBeNull();
	});
});

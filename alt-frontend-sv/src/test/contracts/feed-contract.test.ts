/**
 * Feed API Contract Tests
 *
 * Validates that mock data used in E2E tests conforms to the proto schema.
 * This prevents mock drift: when the proto changes, these tests break before
 * the E2E tests silently pass with stale mock data.
 */

import {
	create,
	fromBinary,
	fromJson,
	toBinary,
	toJson,
} from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import {
	GetAllFeedsResponseSchema,
	GetDetailedFeedStatsResponseSchema,
	GetFeedStatsResponseSchema,
	GetUnreadFeedsResponseSchema,
	MarkAsReadResponseSchema,
	ResolveOgImagesResponseSchema,
} from "$lib/gen/alt/feeds/v2/feeds_pb";
import {
	buildConnectFeedItem,
	buildConnectFeedsResponse,
} from "../../../tests/e2e/fixtures/factories";
import { buildResolveOgImagesResponse } from "../../../tests/e2e/fixtures/mockData";

describe("Feed API Contract", () => {
	it("GetUnreadFeedsResponse conforms to proto schema", () => {
		const mockData = buildConnectFeedsResponse();
		const response = create(GetUnreadFeedsResponseSchema, {
			data: mockData.data.map((f) => ({
				id: f.id,
				title: f.title,
				description: f.description,
				link: f.link,
				published: f.published,
				createdAt: f.createdAt,
				author: f.author,
				articleId: f.articleId,
			})),
			hasMore: mockData.hasMore,
		});

		expect(response.data).toHaveLength(2);
		expect(response.data[0]!.title).toBe("AI Trends");
		expect(response.hasMore).toBe(false);
	});

	it("GetAllFeedsResponse round-trips through proto serialization", () => {
		const original = create(GetAllFeedsResponseSchema, {
			data: [
				{
					id: "feed-1",
					title: "Test Feed",
					description: "Test",
					link: "https://example.com",
					published: "1 hour ago",
					createdAt: new Date().toISOString(),
					author: "Author",
				},
			],
			hasMore: false,
		});

		const binary = toBinary(GetAllFeedsResponseSchema, original);
		const deserialized = fromBinary(GetAllFeedsResponseSchema, binary);

		expect(deserialized.data).toHaveLength(original.data.length);
		expect(deserialized.data[0]!.title).toBe("Test Feed");
		expect(deserialized.hasMore).toBe(false);
	});

	it("FeedItem has required fields", () => {
		const feedItem = buildConnectFeedItem({
			title: "Required Fields Test",
		});

		// All these fields should be present in a well-formed FeedItem
		expect(feedItem.id).toBeDefined();
		expect(feedItem.title).toBeDefined();
		expect(feedItem.description).toBeDefined();
		expect(feedItem.link).toBeDefined();
		expect(feedItem.published).toBeDefined();
		expect(feedItem.createdAt).toBeDefined();
		expect(feedItem.author).toBeDefined();
	});

	it("GetFeedStatsResponse uses bigint for counts", () => {
		const response = create(GetFeedStatsResponseSchema, {
			feedAmount: 10n,
			summarizedFeedAmount: 5n,
		});

		expect(typeof response.feedAmount).toBe("bigint");
		expect(response.feedAmount).toBe(10n);
	});

	it("GetDetailedFeedStatsResponse includes all stat fields", () => {
		const response = create(GetDetailedFeedStatsResponseSchema, {
			feedAmount: 12n,
			articleAmount: 345n,
			unsummarizedFeedAmount: 7n,
		});

		expect(response.feedAmount).toBe(12n);
		expect(response.articleAmount).toBe(345n);
		expect(response.unsummarizedFeedAmount).toBe(7n);
	});

	it("MarkAsReadResponse has message field", () => {
		const response = create(MarkAsReadResponseSchema, {
			message: "Feed marked as read",
		});

		expect(response.message).toBe("Feed marked as read");
	});
});

/**
 * ResolveOgImages carries four outcomes across two lists, and three of them are
 * kinds of *absence* from `images`. That makes its mock uniquely easy to drift:
 * every wrong shape still decodes, so nothing breaks until a reader is shown a
 * blank card — or until the grid re-scrapes a publisher on every scroll.
 *
 * Two different things are pinned below, and they are not the same thing:
 *
 *  - conformance, checked with `fromJson`, because Connect JSON is what the
 *    Playwright route handler actually serves and what the browser's client
 *    actually parses. `create()` would not do: it takes a TS init object,
 *    silently ignores properties the proto does not have, and never sees the
 *    string form an int64 arrives in. `fromJson` rejects both.
 *  - meaning, checked by asserting *membership*. The wire says which of the
 *    four answers it means by which list a feed is in, so a schema-valid mock
 *    can still say the wrong thing. This is exactly what happened when
 *    `unresolved` was added: `{images: []}` kept decoding, but stopped meaning
 *    "asked and refused" and started meaning "never considered", which is the
 *    one answer that licences an immediate re-ask.
 */
describe("ResolveOgImages Contract", () => {
	it("a fixture where every feed resolved conforms to the proto schema", () => {
		const json = buildResolveOgImagesResponse({
			resolved: ["feed-1", "feed-2"],
		});

		const decoded = fromJson(ResolveOgImagesResponseSchema, json);

		expect(decoded.images).toHaveLength(2);
		expect(decoded.images.map((i) => i.feedId)).toEqual(["feed-1", "feed-2"]);
		// The proxy path's last segment must base64-decode to a real upstream
		// URL — `extractImageHost` reads the host out of it to queue loads
		// per-host, and a fixture that is not decodable would silently bypass
		// that queue instead of exercising it.
		expect(decoded.images[0]!.ogImageProxyUrl).toMatch(
			/^\/v1\/images\/proxy\/[^/]+\/[A-Za-z0-9+/=]+$/,
		);
		expect(decoded.unresolved).toHaveLength(0);
	});

	it("a settled fixture puts the feed in `unresolved` rather than only outside `images`", () => {
		const json = buildResolveOgImagesResponse({ settled: ["feed-1"] });

		const decoded = fromJson(ResolveOgImagesResponseSchema, json);

		expect(decoded.images).toHaveLength(0);
		// Membership, not emptiness. `{images: []}` alone would decode just as
		// happily and mean the opposite — see this describe block's note.
		expect(decoded.unresolved.map((u) => u.feedId)).toEqual(["feed-1"]);
		expect(decoded.unresolved[0]!.retryAfterSeconds).toBe(0n);
	});

	it("leaves a feed out of both lists to mean the server never considered it", () => {
		// The fourth outcome has no wire symbol of its own: it is absence from
		// both lists. A builder that helpfully filled in a row for every feed
		// asked about would erase it, so this pins that it does not.
		const json = buildResolveOgImagesResponse({
			resolved: ["feed-1"],
			settled: ["feed-2"],
		});

		const decoded = fromJson(ResolveOgImagesResponseSchema, json);

		expect(decoded.images.map((i) => i.feedId)).toEqual(["feed-1"]);
		expect(decoded.unresolved.map((u) => u.feedId)).toEqual(["feed-2"]);
		expect(
			[...decoded.images, ...decoded.unresolved].some(
				(e) => e.feedId === "feed-3",
			),
		).toBe(false);
	});

	it("carries retry_after_seconds as a JSON string and an in-message bigint", () => {
		// int64 is the trap. protobuf-es types the field as `bigint`, but
		// protobuf's JSON mapping puts 64-bit integers on the wire as *strings*
		// so that a value past 2^53 survives a JSON parser. A fixture written
		// as a numeric literal happens to decode — protobuf JSON accepts both
		// forms — but it is not what the Go server emits, so a mock written
		// that way stops resembling the thing it stands in for.
		const json = buildResolveOgImagesResponse({ failedFor: { "feed-1": 20 } });

		expect(json.unresolved[0]!.retryAfterSeconds).toBe("20");

		const decoded = fromJson(ResolveOgImagesResponseSchema, json);
		expect(typeof decoded.unresolved[0]!.retryAfterSeconds).toBe("bigint");
		expect(decoded.unresolved[0]!.retryAfterSeconds).toBe(20n);

		const roundTripped = fromBinary(
			ResolveOgImagesResponseSchema,
			toBinary(ResolveOgImagesResponseSchema, decoded),
		);
		expect(roundTripped.unresolved[0]!.retryAfterSeconds).toBe(20n);
		expect(toJson(ResolveOgImagesResponseSchema, roundTripped)).toEqual({
			unresolved: [{ feedId: "feed-1", retryAfterSeconds: "20" }],
		});
	});

	it("keeps the row of a zero bar even though proto3 drops the zero itself", () => {
		// proto3 omits default-valued scalars from both encodings, so a bar of
		// zero vanishes from the JSON and occupies no bytes in the binary. What
		// does NOT vanish is the `UnresolvedOgImage` row carrying it — and the
		// row is the whole signal, because `outcomeFor` reads membership of the
		// `unresolved` map, never the presence of the field. That is why this
		// contract survives default omission where a `status` enum would not.
		const json = buildResolveOgImagesResponse({ settled: ["feed-1"] });
		const decoded = fromJson(ResolveOgImagesResponseSchema, json);

		const roundTripped = fromBinary(
			ResolveOgImagesResponseSchema,
			toBinary(ResolveOgImagesResponseSchema, decoded),
		);

		expect(roundTripped.unresolved).toHaveLength(1);
		expect(roundTripped.unresolved[0]!.feedId).toBe("feed-1");
		expect(roundTripped.unresolved[0]!.retryAfterSeconds).toBe(0n);
		// The zero is gone from the wire form; the row it belonged to is not.
		expect(toJson(ResolveOgImagesResponseSchema, roundTripped)).toEqual({
			unresolved: [{ feedId: "feed-1" }],
		});
	});

	it("rejects a fixture carrying a field the proto does not have", () => {
		// The drift this whole file exists to catch: a renamed or invented key
		// that a hand-written mock keeps serving long after the proto moved on.
		expect(() =>
			fromJson(ResolveOgImagesResponseSchema, {
				images: [{ feedId: "feed-1", ogImageUrl: "https://img/x.png" }],
			}),
		).toThrow(/unknown/i);
	});
});

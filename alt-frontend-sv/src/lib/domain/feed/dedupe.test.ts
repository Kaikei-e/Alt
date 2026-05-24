import { describe, expect, it } from "vitest";

import { appendUniqueById } from "./dedupe";

describe("appendUniqueById", () => {
	// Meilisearch hybrid search (semanticRatio > 0) does not produce a stable
	// total ordering across offset windows: the BM25 + vector score fusion can
	// shuffle items near a page boundary, so the same article_id can appear in
	// both page N and page N+1. The keyed each block on /feeds/search uses
	// feed.id as its identity, so an undeduped concat trips
	// https://svelte.dev/e/each_key_duplicate and freezes the list at the
	// previous size. appendUniqueById is the guard we run before every concat
	// to keep the list monotonically growing and crash-free.
	it("appends only items whose id is not already present", () => {
		const existing = [{ id: "a" }, { id: "b" }, { id: "c" }];
		const incoming = [{ id: "c" }, { id: "d" }, { id: "b" }, { id: "e" }];

		const merged = appendUniqueById(existing, incoming);

		expect(merged).toEqual([
			{ id: "a" },
			{ id: "b" },
			{ id: "c" },
			{ id: "d" },
			{ id: "e" },
		]);
	});

	it("returns the original array when every incoming id is already present", () => {
		const existing = [{ id: "a" }, { id: "b" }];
		const incoming = [{ id: "a" }, { id: "b" }];

		const merged = appendUniqueById(existing, incoming);

		expect(merged).toEqual(existing);
		// Same length signals to the caller that load-more produced no progress
		// and `hasMore` should flip to false to stop the infinite scroll loop.
		expect(merged.length).toBe(existing.length);
	});

	it("deduplicates internally within the incoming page too", () => {
		const existing: { id: string }[] = [];
		const incoming = [{ id: "a" }, { id: "a" }, { id: "b" }];

		const merged = appendUniqueById(existing, incoming);

		expect(merged).toEqual([{ id: "a" }, { id: "b" }]);
	});

	it("dedupes empty-string ids the same as any other id", () => {
		// RenderFeed.id falls back to "" when both article_id and link are
		// missing. The keyed each block cannot host two siblings under the same
		// key, so the helper must collapse multiple "" rows down to one to
		// avoid reintroducing the each_key_duplicate crash.
		const existing = [{ id: "" }];
		const incoming = [{ id: "" }, { id: "x" }];

		const merged = appendUniqueById(existing, incoming);

		expect(merged).toEqual([{ id: "" }, { id: "x" }]);
	});
});

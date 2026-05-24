/**
 * appendUniqueById appends `incoming` to `existing`, skipping any item whose
 * `id` already appears in `existing` or earlier in `incoming`. It exists as
 * the guard /feeds/search runs before every concat from the cursor-paginated
 * SearchFeeds RPC.
 *
 * Why this is necessary:
 *   Meilisearch hybrid search (semanticRatio > 0) fuses BM25 and vector
 *   similarity scores at query time. The fused score is not perfectly stable
 *   across paginated requests — items sitting on a page boundary can swap
 *   places between offset=N and offset=N+limit, so the same article_id can
 *   appear in both pages. The Archive Desk search keys the each block by
 *   `feed.id`, which means an undeduped concat trips
 *   `https://svelte.dev/e/each_key_duplicate` and the result list freezes at
 *   its previous size (visible symptom: "20 items loaded, load-more does
 *   nothing").
 *
 * Empty-string ids are deduped just like any other id: the keyed each block
 * cannot hold two siblings under the same key, so keeping multiple "" rows
 * would reintroduce the very crash this helper exists to prevent.
 */
export function appendUniqueById<T extends { id: string }>(
	existing: T[],
	incoming: T[],
): T[] {
	const seen = new Set<string>(existing.map((item) => item.id));

	const result = [...existing];
	for (const item of incoming) {
		if (seen.has(item.id)) {
			continue;
		}
		seen.add(item.id);
		result.push(item);
	}
	return result;
}

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

/**
 * appendUniqueFeeds is {@link appendUniqueById} plus a second identity: a feed
 * is skipped when its `id` OR its `normalizedUrl` already appears.
 *
 * The URL half matters because the same article can arrive under two different
 * ids. `/feeds` renders a server-loaded first screen and then fetches page one
 * from the client, and those are two separate reads of the wire — a backend
 * that mints row ids per query answers the second one with different ids for
 * the same articles. Deduping on `id` alone let both copies through, and the
 * reader saw every headline on their first screen twice. (The mobile list this
 * grid replaced deduped on `normalizedUrl` for exactly this reason.)
 *
 * The `id` half still has to be there on its own: the keyed `{#each}` that
 * renders these is keyed by `id`, and two siblings under one key is a hard
 * runtime error, not a warning — https://svelte.dev/e/each_key_duplicate.
 *
 * First occurrence wins, and the order of `existing` is never disturbed.
 */
export function appendUniqueFeeds<
	T extends { id: string; normalizedUrl: string },
>(existing: T[], incoming: T[]): T[] {
	const seenIds = new Set<string>(existing.map((item) => item.id));
	const seenUrls = new Set<string>(existing.map((item) => item.normalizedUrl));

	const result = [...existing];
	for (const item of incoming) {
		if (seenIds.has(item.id) || seenUrls.has(item.normalizedUrl)) {
			continue;
		}
		seenIds.add(item.id);
		seenUrls.add(item.normalizedUrl);
		result.push(item);
	}
	return result;
}

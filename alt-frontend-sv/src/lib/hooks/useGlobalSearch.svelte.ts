/**
 * Headless composable for Global Federated Search.
 * Follows the useKnowledgeHome.svelte.ts Runes pattern.
 */

import { Code, ConnectError } from "@connectrpc/connect";
import { goto } from "$app/navigation";
import { createClientTransport } from "$lib/connect";
import {
	searchEverything,
	type GlobalSearchResult,
	type GlobalSearchOptions,
} from "$lib/connect/global_search";

export function useGlobalSearch() {
	let query = $state("");
	let loading = $state(false);
	let error = $state<Error | null>(null);
	let result = $state<GlobalSearchResult | null>(null);

	const hasResults = $derived(
		result !== null &&
			((result.articleSection?.hits?.length ?? 0) > 0 ||
				(result.recapSection?.hits?.length ?? 0) > 0 ||
				(result.tagSection?.hits?.length ?? 0) > 0),
	);

	const degradedSections = $derived(result?.degradedSections ?? []);

	const search = async (q: string, options?: GlobalSearchOptions) => {
		const trimmed = q.trim();
		if (!trimmed) return;

		try {
			query = trimmed;
			loading = true;
			error = null;
			const transport = createClientTransport();
			const searchResult = await searchEverything(transport, trimmed, options);
			result = searchResult;
		} catch (err) {
			if (err instanceof ConnectError) {
				if (err.code === Code.Unauthenticated) {
					goto("/login");
					return;
				}
			}
			error = err instanceof Error ? err : new Error("Unknown error");
			result = null;
		} finally {
			loading = false;
		}
	};

	const clear = () => {
		query = "";
		result = null;
		error = null;
		loading = false;
	};

	return {
		get query() {
			return query;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		get result() {
			return result;
		},
		get hasResults() {
			return hasResults;
		},
		get degradedSections() {
			return degradedSections;
		},
		search,
		clear,
	};
}

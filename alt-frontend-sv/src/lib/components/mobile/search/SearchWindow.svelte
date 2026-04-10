<script lang="ts">
import { onMount } from "svelte";
import * as v from "valibot";
import { searchFeedsClient } from "$lib/api/client";
import type { SearchFeedItem, SearchQuery } from "$lib/schema/search";
import { transformFeedSearchResult } from "$lib/utils/transformFeedSearchResult";

interface Props {
	searchQuery: SearchQuery;
	autoSearch?: boolean;
	setSearchQuery: (query: SearchQuery) => void;
	setFeedResults: (results: SearchFeedItem[]) => void;
	setCursor: (cursor: string | null) => void;
	setHasMore: (hasMore: boolean) => void;
	isLoading: boolean;
	setIsLoading: (loading: boolean) => void;
	setSearchTime?: (time: number) => void;
}

const {
	searchQuery,
	autoSearch = false,
	setSearchQuery,
	setFeedResults,
	setCursor,
	setHasMore,
	isLoading,
	setIsLoading,
	setSearchTime,
}: Props = $props();

onMount(() => {
	if (autoSearch && searchQuery.query?.trim()) {
		handleSearch();
	}
});

let error = $state<string | null>(null);
let validationError = $state<string | null>(null);
let abortController: AbortController | null = $state(null);

const searchQuerySchema = v.object({
	query: v.pipe(
		v.string("Please enter a search query"),
		v.trim(),
		v.nonEmpty("Please enter a search query"),
		v.minLength(2, "Search query must be at least 2 characters"),
		v.maxLength(100, "Search query must be at most 100 characters"),
	),
});

function validateQuery(queryText: string): string | null {
	const trimmed = queryText.trim();

	if (!trimmed) {
		return "Please enter a search query";
	}

	if (trimmed.length < 2) {
		return "Search query must be at least 2 characters";
	}

	const result = v.safeParse(searchQuerySchema, { query: trimmed });
	if (!result.success) {
		return result.issues?.[0]?.message || "Please enter a valid search query";
	}

	return null;
}

const handleSearch = async () => {
	if (isLoading) return;

	if (abortController) {
		abortController.abort();
	}

	abortController = new AbortController();

	const startTime = Date.now();
	setIsLoading(true);
	error = null;
	validationError = null;

	try {
		setFeedResults([]);
		setCursor(null);
		setHasMore(false);

		const validationResult = validateQuery(searchQuery.query || "");

		if (validationResult) {
			validationError = validationResult;
			setIsLoading(false);
			return;
		}

		const validatedQuery = searchQuery.query.trim();

		const searchResult = await searchFeedsClient(validatedQuery);

		if (abortController?.signal.aborted) {
			setIsLoading(false);
			return;
		}

		if (searchResult.error) {
			error = searchResult.error;
			setIsLoading(false);
			return;
		}

		const transformedResults = transformFeedSearchResult(searchResult);

		setFeedResults(transformedResults);
		const nextCursorStr =
			searchResult.next_cursor !== null &&
			searchResult.next_cursor !== undefined
				? String(searchResult.next_cursor)
				: null;
		setCursor(nextCursorStr);
		const hasMoreValue =
			searchResult.has_more ?? searchResult.next_cursor !== null;
		setHasMore(hasMoreValue);

		const searchTime = Date.now() - startTime;
		setSearchTime?.(searchTime);
	} catch (err) {
		if (abortController?.signal.aborted) {
			setIsLoading(false);
			return;
		}
		console.error("Search error:", err);
		error =
			err instanceof Error ? err.message : "Search failed. Please try again.";
	} finally {
		if (!abortController?.signal.aborted) {
			setIsLoading(false);
		}
	}
};

const handleFormSubmit = (e: Event) => {
	e.preventDefault();
	const validationResult = validateQuery(searchQuery.query || "");
	if (validationResult) {
		validationError = validationResult;
		return;
	}
	handleSearch();
};

const handleKeyPress = (e: KeyboardEvent) => {
	if (e.key === "Enter") {
		e.preventDefault();
		const validationResult = validateQuery(searchQuery.query || "");

		if (validationResult) {
			validationError = validationResult;
			return;
		}

		handleSearch();
	}
};

const handleInputChange = (e: Event) => {
	const target = e.target as HTMLInputElement;
	const newQuery = target.value;
	setSearchQuery({ query: newQuery });

	if (error) error = null;

	if (newQuery.trim().length >= 2) {
		validationError = null;
	}
};
</script>

<div data-testid="search-window">
	<form onsubmit={handleFormSubmit}>
		<div class="flex flex-col gap-4">
			<div class="w-full">
				<label
					for="search-input"
					class="search-label"
				>
					Search Query
				</label>
				<input
					id="search-input"
					data-testid="search-input"
					type="text"
					value={searchQuery.query || ""}
					oninput={handleInputChange}
					onkeydown={handleKeyPress}
					placeholder="e.g. AI, technology, startup..."
					disabled={isLoading}
					class="archive-input-mobile"
					class:archive-input-mobile--error={!!validationError}
				/>
			</div>

			<button
				type="submit"
				class="archive-btn-mobile"
				disabled={isLoading || !!validationError}
			>
				{#if isLoading}
					<span class="loading-pulse"></span>
					<span class="archive-btn-text">Searching...</span>
				{:else}
					SEARCH
				{/if}
			</button>

			{#if validationError}
				<div class="error-stripe">{validationError}</div>
			{/if}

			{#if error}
				<div class="error-stripe">{error}</div>
			{/if}
		</div>
	</form>
</div>

<style>
	.search-label {
		display: block;
		margin-bottom: 0.5rem;
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.archive-input-mobile {
		width: 100%;
		padding: 0.75rem;
		font-family: var(--font-body);
		font-size: 1rem;
		color: var(--alt-charcoal);
		background: var(--surface-bg);
		border: 1px solid var(--surface-border);
		border-radius: 0;
		outline: none;
		transition: border-color 0.15s;
	}

	.archive-input-mobile:focus {
		border-color: var(--alt-charcoal);
	}

	.archive-input-mobile::placeholder {
		color: var(--alt-ash);
	}

	.archive-input-mobile--error {
		border-color: var(--alt-terracotta);
	}

	.archive-input-mobile--error:focus {
		border-color: var(--alt-terracotta);
	}

	.archive-btn-mobile {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		width: 100%;
		min-height: 44px;
		padding: 0.75rem;
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.archive-btn-mobile:hover:not(:disabled) {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.archive-btn-mobile:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.archive-btn-text {
		font-style: italic;
	}

	.error-stripe {
		padding: 0.5rem 0.75rem;
		border-left: 3px solid var(--alt-terracotta);
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-terracotta);
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: currentColor;
		animation: pulse 1.2s ease-in-out infinite;
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
		}
		50% {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.loading-pulse {
			animation: none;
			opacity: 1;
		}
	}
</style>

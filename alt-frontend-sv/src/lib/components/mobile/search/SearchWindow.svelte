<script lang="ts">
import { Loader } from "@lucide/svelte";
import * as v from "valibot";
import { searchFeedsClient } from "$lib/api/client";
import { Button } from "$lib/components/ui/button";
import { Input } from "$lib/components/ui/input";
import type { SearchFeedItem, SearchQuery } from "$lib/schema/search";
import { transformFeedSearchResult } from "$lib/utils/transformFeedSearchResult";

interface Props {
	searchQuery: SearchQuery;
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
	setSearchQuery,
	setFeedResults,
	setCursor,
	setHasMore,
	isLoading,
	setIsLoading,
	setSearchTime,
}: Props = $props();

let error = $state<string | null>(null);
let validationError = $state<string | null>(null);
let abortController: AbortController | null = $state(null);

// Search query schema for validation
const searchQuerySchema = v.object({
	query: v.pipe(
		v.string("Please enter a search query"),
		v.trim(),
		v.nonEmpty("Please enter a search query"),
		v.minLength(2, "Search query must be at least 2 characters"),
		v.maxLength(100, "Search query must be at most 100 characters"),
	),
});

// Validate query function
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

	// Cancel any in-flight request
	if (abortController) {
		abortController.abort();
	}

	abortController = new AbortController();

	const startTime = Date.now();
	setIsLoading(true);
	error = null;
	validationError = null;

	try {
		// 1. Clear previous results
		setFeedResults([]);
		setCursor(null);
		setHasMore(false);

		// 2. Validate input
		const validationResult = validateQuery(searchQuery.query || "");

		if (validationResult) {
			validationError = validationResult;
			setIsLoading(false);
			return;
		}

		const validatedQuery = searchQuery.query.trim();

		// 3. Call API
		const searchResult = await searchFeedsClient(validatedQuery);

		// Check if aborted
		if (abortController?.signal.aborted) {
			setIsLoading(false);
			return;
		}

		if (searchResult.error) {
			error = searchResult.error;
			setIsLoading(false);
			return;
		}

		// 4. Transform results
		const transformedResults = transformFeedSearchResult(searchResult);

		console.log('[SearchWindow:handleSearch] API response', {
			resultsCount: transformedResults.length,
			nextCursor: searchResult.next_cursor,
			hasMore: searchResult.has_more,
			rawNextCursor: searchResult.next_cursor,
		});

		// 5. Update state
		setFeedResults(transformedResults);
		// Convert cursor to string for state management (offset as string)
		const nextCursorStr = searchResult.next_cursor !== null &&
			searchResult.next_cursor !== undefined
			? String(searchResult.next_cursor)
			: null;
		setCursor(nextCursorStr);
		// Use has_more from response if available, otherwise fall back to next_cursor check
		const hasMoreValue = searchResult.has_more ?? (searchResult.next_cursor !== null);
		setHasMore(hasMoreValue);

		console.log('[SearchWindow:handleSearch] State updated', {
			cursor: nextCursorStr,
			hasMore: hasMoreValue,
		});

		// 6. Track search time
		const searchTime = Date.now() - startTime;
		setSearchTime?.(searchTime);
	} catch (err) {
		if (abortController?.signal.aborted) {
			setIsLoading(false);
			return; // Ignore abort errors
		}
		console.error("Search error:", err);
		error =
			err instanceof Error ? err.message : "Search failed. Please try again.";
	} finally {
		// Always reset loading state if not aborted
		if (!abortController?.signal.aborted) {
			setIsLoading(false);
		}
	}
};

const handleFormSubmit = (e: Event) => {
	e.preventDefault();
	// Always validate on form submit
	const validationResult = validateQuery(searchQuery.query || "");
	if (validationResult) {
		validationError = validationResult;
		return;
	}
	// Only proceed with search if validation passes
	handleSearch();
};

const handleKeyPress = (e: KeyboardEvent) => {
	if (e.key === "Enter") {
		e.preventDefault();
		// Directly call the form submission logic
		const validationResult = validateQuery(searchQuery.query || "");

		if (validationResult) {
			validationError = validationResult;
			return;
		}

		// Only proceed with search if validation passes
		handleSearch();
	}
};

const handleInputChange = (e: Event) => {
	const target = e.target as HTMLInputElement;
	const newQuery = target.value;
	setSearchQuery({ query: newQuery });

	// Clear API errors when user starts typing
	if (error) error = null;

	// Clear validation error when user types enough characters
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
					class="block mb-2 text-sm font-medium"
					style="color: var(--text-secondary);"
				>
					Search Query
				</label>
				<Input
					id="search-input"
					data-testid="search-input"
					type="text"
					value={searchQuery.query || ""}
					oninput={handleInputChange}
					onkeydown={handleKeyPress}
					placeholder="e.g. AI, technology, startup..."
					disabled={isLoading}
					class={validationError
						? "border-[#dc2626] focus-visible:border-[#dc2626]"
						: ""}
				/>
			</div>

			<Button
				type="submit"
				class="w-full rounded-full min-h-[48px] font-semibold text-base transition-all duration-200 hover:scale-[1.02] active:scale-[0.98] disabled:opacity-60"
				style="
					background: var(--alt-primary);
					color: black;
				"
				disabled={isLoading || !!validationError}
			>
				{#if isLoading}
					<div class="flex items-center gap-2">
						<Loader class="h-4 w-4 animate-spin" />
						<span>Searching...</span>
					</div>
				{:else}
					Search
				{/if}
			</Button>

			{#if validationError}
				<p
					class="text-center text-sm font-medium"
					style="color: #dc2626;"
				>
					{validationError}
				</p>
			{/if}

			{#if error}
				<p
					class="text-center text-sm font-medium"
					style="color: #dc2626;"
				>
					{error}
				</p>
			{/if}
		</div>
	</form>
</div>


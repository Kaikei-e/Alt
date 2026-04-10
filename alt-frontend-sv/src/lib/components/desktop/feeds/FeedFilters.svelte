<script lang="ts">
import type { ConnectFeedSource } from "$lib/connect/feeds";
import FeedSourceExcludeFilter from "./FeedSourceExcludeFilter.svelte";

interface Props {
	unreadOnly?: boolean;
	sortBy?: string;
	excludedFeedLinkIds?: string[];
	feedSources?: ConnectFeedSource[];
	onFilterChange: (filters: {
		unreadOnly: boolean;
		sortBy: string;
		excludedFeedLinkIds: string[];
	}) => void;
}

let {
	unreadOnly = false,
	sortBy = "date_desc",
	excludedFeedLinkIds = [],
	feedSources = [],
	onFilterChange,
}: Props = $props();

let localUnreadOnly = $state(false);
let localSortBy = $state("date_desc");
let localExcludedFeedLinkIds = $state<string[]>([]);

$effect.pre(() => {
	localUnreadOnly = unreadOnly;
	localSortBy = sortBy;
	localExcludedFeedLinkIds = excludedFeedLinkIds ?? [];
});

function handleUnreadChange(event: Event) {
	const target = event.target as HTMLInputElement;
	localUnreadOnly = target.checked;
	onFilterChange({
		unreadOnly: localUnreadOnly,
		sortBy: localSortBy,
		excludedFeedLinkIds: localExcludedFeedLinkIds,
	});
}

function handleSortChange(event: Event) {
	const target = event.target as HTMLSelectElement;
	localSortBy = target.value;
	onFilterChange({
		unreadOnly: localUnreadOnly,
		sortBy: localSortBy,
		excludedFeedLinkIds: localExcludedFeedLinkIds,
	});
}

function handleExclude(feedLinkIds: string[]) {
	localExcludedFeedLinkIds = feedLinkIds;
	onFilterChange({
		unreadOnly: localUnreadOnly,
		sortBy: localSortBy,
		excludedFeedLinkIds: localExcludedFeedLinkIds,
	});
}

function handleClearExclusion() {
	localExcludedFeedLinkIds = [];
	onFilterChange({
		unreadOnly: localUnreadOnly,
		sortBy: localSortBy,
		excludedFeedLinkIds: [],
	});
}
</script>

<div class="filter-bar">
	<div class="flex items-center gap-3">
		<div class="filter-group">
			<input
				type="checkbox"
				id="unread-only"
				checked={localUnreadOnly}
				onchange={handleUnreadChange}
				class="filter-checkbox"
			/>
			<label for="unread-only" class="filter-label">Unread Only</label>
		</div>

		<div class="filter-divider" aria-hidden="true"></div>

		<FeedSourceExcludeFilter
			sources={feedSources}
			excludedFeedLinkIds={localExcludedFeedLinkIds}
			onExclude={handleExclude}
			onClearExclusion={handleClearExclusion}
		/>
	</div>

	<div class="filter-group">
		<select
			value={localSortBy}
			onchange={handleSortChange}
			class="filter-select"
		>
			<option value="date_desc">Date (Newest)</option>
			<option value="date_asc">Date (Oldest)</option>
			<option value="title_asc">Title (A-Z)</option>
			<option value="title_desc">Title (Z-A)</option>
		</select>
	</div>
</div>

<style>
	.filter-bar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.6rem 0;
		border-bottom: 1px solid var(--surface-border);
		margin-bottom: 0.75rem;
	}

	.filter-group {
		display: flex;
		align-items: center;
		gap: 0.4rem;
	}

	.filter-checkbox {
		width: 1rem;
		height: 1rem;
		cursor: pointer;
		accent-color: var(--alt-primary);
	}

	.filter-label {
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-charcoal);
		cursor: pointer;
	}

	.filter-divider {
		width: 1px;
		height: 1rem;
		background: var(--surface-border);
		flex-shrink: 0;
	}

	.filter-select {
		width: 160px;
		height: 2rem;
		padding: 0 0.5rem;
		border: 1px solid var(--surface-border);
		background: transparent;
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-charcoal);
		cursor: pointer;
	}

	.filter-select:focus {
		border-color: var(--alt-charcoal);
		outline: none;
	}
</style>

<script lang="ts">
import { Filter, ArrowUpDown } from "@lucide/svelte";
import type { ConnectFeedSource } from "$lib/connect/feeds";
import FeedSourceExcludeFilter from "./FeedSourceExcludeFilter.svelte";

interface Props {
	unreadOnly?: boolean;
	sortBy?: string;
	excludedFeedLinkId?: string | null;
	feedSources?: ConnectFeedSource[];
	onFilterChange: (filters: { unreadOnly: boolean; sortBy: string; excludedFeedLinkId: string | null }) => void;
}

let {
	unreadOnly = false,
	sortBy = "date_desc",
	excludedFeedLinkId = null,
	feedSources = [],
	onFilterChange,
}: Props = $props();

let localUnreadOnly = $state(false);
let localSortBy = $state("date_desc");
let localExcludedFeedLinkId = $state<string | null>(null);

$effect.pre(() => {
	localUnreadOnly = unreadOnly;
	localSortBy = sortBy;
	localExcludedFeedLinkId = excludedFeedLinkId ?? null;
});

function handleUnreadChange(event: Event) {
	const target = event.target as HTMLInputElement;
	localUnreadOnly = target.checked;
	onFilterChange({ unreadOnly: localUnreadOnly, sortBy: localSortBy, excludedFeedLinkId: localExcludedFeedLinkId });
}

function handleSortChange(event: Event) {
	const target = event.target as HTMLSelectElement;
	localSortBy = target.value;
	onFilterChange({ unreadOnly: localUnreadOnly, sortBy: localSortBy, excludedFeedLinkId: localExcludedFeedLinkId });
}

function handleExclude(feedLinkId: string) {
	localExcludedFeedLinkId = feedLinkId;
	onFilterChange({ unreadOnly: localUnreadOnly, sortBy: localSortBy, excludedFeedLinkId: localExcludedFeedLinkId });
}

function handleClearExclusion() {
	localExcludedFeedLinkId = null;
	onFilterChange({ unreadOnly: localUnreadOnly, sortBy: localSortBy, excludedFeedLinkId: null });
}
</script>

<div class="flex items-center justify-between gap-4 mb-6 p-4 border border-[var(--surface-border)] bg-white">
	<!-- Filter section -->
	<div class="flex items-center gap-3">
		<Filter class="h-4 w-4 text-[var(--text-secondary)]" />
		<div class="flex items-center gap-2">
			<input
				type="checkbox"
				id="unread-only"
				checked={localUnreadOnly}
				onchange={handleUnreadChange}
				class="w-4 h-4 cursor-pointer"
			/>
			<label for="unread-only" class="text-sm text-[var(--text-primary)] cursor-pointer">
				Unread Only
			</label>
		</div>

		<!-- Exclude source filter -->
		<div class="border-l border-[var(--surface-border)] pl-3">
			<FeedSourceExcludeFilter
				sources={feedSources}
				excludedSourceId={localExcludedFeedLinkId}
				onExclude={handleExclude}
				onClearExclusion={handleClearExclusion}
			/>
		</div>
	</div>

	<!-- Sort section -->
	<div class="flex items-center gap-3">
		<ArrowUpDown class="h-4 w-4 text-[var(--text-secondary)]" />
		<select
			value={localSortBy}
			onchange={handleSortChange}
			class="w-[180px] h-9 px-3 border border-[var(--surface-border)] bg-white text-sm cursor-pointer"
		>
			<option value="date_desc">Date (Newest)</option>
			<option value="date_asc">Date (Oldest)</option>
			<option value="title_asc">Title (A-Z)</option>
			<option value="title_desc">Title (Z-A)</option>
		</select>
	</div>
</div>

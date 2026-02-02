<script lang="ts">
import { Filter, ArrowUpDown } from "@lucide/svelte";

interface Props {
	unreadOnly?: boolean;
	sortBy?: string;
	onFilterChange: (filters: { unreadOnly: boolean; sortBy: string }) => void;
}

let {
	unreadOnly = false,
	sortBy = "date_desc",
	onFilterChange,
}: Props = $props();

let localUnreadOnly = $state(false);
let localSortBy = $state("date_desc");

$effect.pre(() => {
	localUnreadOnly = unreadOnly;
	localSortBy = sortBy;
});

function handleUnreadChange(event: Event) {
	const target = event.target as HTMLInputElement;
	localUnreadOnly = target.checked;
	onFilterChange({ unreadOnly: localUnreadOnly, sortBy: localSortBy });
}

function handleSortChange(event: Event) {
	const target = event.target as HTMLSelectElement;
	localSortBy = target.value;
	onFilterChange({ unreadOnly: localUnreadOnly, sortBy: localSortBy });
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

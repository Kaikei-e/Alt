<script lang="ts">
import { BirdIcon, Search } from "@lucide/svelte";
import { goto } from "$app/navigation";

let query = $state("");

function handleSearch() {
	const trimmed = query.trim();
	if (trimmed) {
		goto(`/feeds/search?q=${encodeURIComponent(trimmed)}`);
	}
}

function handleAsk() {
	const trimmed = query.trim();
	if (trimmed) {
		goto(`/augur?q=${encodeURIComponent(trimmed)}`);
	} else {
		goto("/augur");
	}
}

function handleKeydown(e: KeyboardEvent) {
	if (e.key === "Enter") {
		handleSearch();
	}
}
</script>

<div
	class="relative flex items-center gap-2 px-4 py-2 bg-[var(--surface-bg)] border-b border-[var(--surface-border)]"
>
	<div class="relative flex-1">
		<input
			type="text"
			bind:value={query}
			onkeydown={handleKeydown}
			placeholder="Search articles or ask a question..."
			class="w-full px-3 py-2 pl-9 text-sm rounded-lg bg-[var(--surface-hover)] text-[var(--text-primary)] placeholder:text-[var(--text-secondary)] border border-transparent focus:border-[var(--accent-primary)] focus:outline-none transition-colors"
		/>
		<Search
			class="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-[var(--text-secondary)]"
		/>
	</div>
	<button
		type="button"
		onclick={handleAsk}
		class="inline-flex items-center gap-1.5 px-3 py-2 text-sm rounded-lg text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
		title="Ask Augur"
		aria-label="Ask Augur"
	>
		<BirdIcon class="h-4 w-4" />
		<span class="hidden sm:inline">Ask</span>
	</button>
</div>

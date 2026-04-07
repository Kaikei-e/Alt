<script lang="ts">
import { goto } from "$app/navigation";
import { BirdIcon, Search } from "@lucide/svelte";

let {
	onAsk,
}: {
	onAsk?: (query: string) => void;
} = $props();

let query = $state("");

function handleSearch() {
	const trimmed = query.trim();
	if (trimmed) {
		goto(`/search?q=${encodeURIComponent(trimmed)}`);
	}
}

function handleAsk() {
	const trimmed = query.trim();
	if (onAsk) {
		onAsk(trimmed);
		return;
	}
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

<div class="px-4 py-3">
	<div class="relative flex items-center gap-2">
		<div class="relative flex-1">
			<input
				type="text"
				bind:value={query}
				onkeydown={handleKeydown}
				placeholder="Search across articles, recaps, and tags"
				class="w-full rounded-xl border border-[var(--surface-border)] bg-[var(--surface-2)] px-4 py-2.5 pl-9 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:border-[var(--interactive-text)] focus:ring-2 focus:ring-[var(--interactive-text)]/20 focus:outline-none transition-colors"
				style="min-height: 44px;"
			/>
			<Search
				class="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-[var(--text-tertiary)]"
			/>
		</div>
		<button
			type="button"
			onclick={handleAsk}
			class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--surface-border)] px-2.5 py-1.5 text-sm font-medium text-[var(--interactive-text)] transition-colors active:bg-[var(--surface-hover)]"
			style="min-height: 44px;"
			title="Ask Augur"
			aria-label="Ask Augur"
		>
			<BirdIcon class="h-[18px] w-[18px]" />
			<span class="hidden sm:inline">Ask</span>
		</button>
	</div>
</div>

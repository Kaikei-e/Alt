<script lang="ts">
import { browser } from "$app/environment";
import { BirdIcon, Search } from "@lucide/svelte";
import { goto } from "$app/navigation";

interface Props {
	query?: string;
	onSearchSubmit?: (query: string) => void;
	onSearchClear?: () => void;
	onAsk?: (query: string) => void;
}

const {
	query: initialQuery = "",
	onSearchSubmit,
	onSearchClear,
	onAsk,
}: Props = $props();

let query = $state("");
let recentQueries = $state<string[]>([]);

$effect(() => {
	query = initialQuery;
});

if (browser) {
	const stored = window.localStorage.getItem("knowledge-home-recent-queries");
	if (stored) {
		try {
			recentQueries = JSON.parse(stored) as string[];
		} catch {
			recentQueries = [];
		}
	}
}

function saveRecent(trimmed: string) {
	if (!browser) return;
	recentQueries = [trimmed, ...recentQueries.filter((item) => item !== trimmed)].slice(
		0,
		5,
	);
	window.localStorage.setItem(
		"knowledge-home-recent-queries",
		JSON.stringify(recentQueries),
	);
}

function handleSearch() {
	const trimmed = query.trim();
	if (trimmed) {
		saveRecent(trimmed);
		if (onSearchSubmit) {
			onSearchSubmit(trimmed);
			return;
		}
		goto(`/feeds/search?q=${encodeURIComponent(trimmed)}`);
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

function clearSearch() {
	query = "";
	onSearchClear?.();
}

function handleKeydown(e: KeyboardEvent) {
	if (e.key === "Enter") {
		handleSearch();
	}
}
</script>

<div class="space-y-3 bg-[var(--surface-bg)] px-4 py-3">
	<div class="relative flex items-center gap-2">
		<div class="relative flex-1">
			<input
				type="text"
				bind:value={query}
				onkeydown={handleKeydown}
				placeholder="Search articles or ask a question..."
				class="w-full rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] px-4 py-2.5 pl-9 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-secondary)] shadow-[var(--shadow-sm)] focus:border-[var(--accent-primary)] focus:ring-2 focus:ring-[var(--accent-primary,var(--interactive-text))]/20 focus:outline-none transition-colors"
			/>
			<Search
				class="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-[var(--text-secondary)]"
			/>
		</div>
		{#if query.trim()}
			<button
				type="button"
				onclick={clearSearch}
				class="rounded-lg px-3 py-2 text-sm text-[var(--text-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--text-primary)] transition-colors"
			>
				Clear
			</button>
		{/if}
		<button
			type="button"
			onclick={handleAsk}
			class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--surface-border)] px-3 py-2 text-sm font-medium text-[var(--interactive-text)] hover:bg-[var(--surface-hover)] hover:text-[var(--interactive-text-hover)] transition-colors"
			title="Ask Augur"
			aria-label="Ask Augur"
		>
			<BirdIcon class="h-4 w-4" />
			<span class="hidden sm:inline">Ask</span>
		</button>
	</div>

	{#if recentQueries.length > 0}
		<div class="flex flex-wrap items-center gap-2">
			<span class="text-xs uppercase tracking-wider text-[var(--text-secondary)]">
				Recent
			</span>
			{#each recentQueries as recent}
				<button
					type="button"
					class="rounded-full bg-[var(--surface-hover)] border border-transparent px-3 py-1 text-xs text-[var(--text-secondary)] hover:bg-[var(--action-surface)] hover:text-[var(--text-primary)]"
					onclick={() => {
						query = recent;
						onSearchSubmit?.(recent);
					}}
				>
					{recent}
				</button>
			{/each}
		</div>
	{/if}
</div>

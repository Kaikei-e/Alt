<script lang="ts">
import { BirdIcon, Search } from "@lucide/svelte";
import { browser } from "$app/env";
import { goto } from "$app/navigation";

interface Props {
	query?: string;
	onSearchSubmit?: (query: string) => void;
	onSearchClear?: () => void;
	onAsk?: (query: string) => void;
}

let {
	query = $bindable(""),
	onSearchSubmit,
	onSearchClear,
	onAsk,
}: Props = $props();

let recentQueries = $state<string[]>([]);

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
	recentQueries = [
		trimmed,
		...recentQueries.filter((item) => item !== trimmed),
	].slice(0, 5);
	try {
		window.localStorage.setItem(
			"knowledge-home-recent-queries",
			JSON.stringify(recentQueries),
		);
	} catch {
		// Safari private mode / quota exceeded — keep in-memory recentQueries
	}
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

<div class="intent-box">
	<div class="intent-row">
		<div class="intent-input-wrap">
			<input
				type="text"
				bind:value={query}
				onkeydown={handleKeydown}
				placeholder="Search articles or ask a question..."
				class="intent-input"
			/>
			<span class="intent-icon"><Search class="h-4 w-4" /></span>
		</div>
		{#if query.trim()}
			<button type="button" onclick={clearSearch} class="intent-btn intent-btn--clear">
				Clear
			</button>
		{/if}
		<button
			type="button"
			onclick={handleAsk}
			class="intent-btn intent-btn--ask"
			title="Ask Augur"
			aria-label="Ask Augur"
		>
			<BirdIcon class="h-4 w-4" />
			<span class="hidden sm:inline">Ask</span>
		</button>
	</div>

	{#if recentQueries.length > 0}
		<div class="recent-row">
			<span class="recent-label">RECENT</span>
			{#each recentQueries as recent}
				<button
					type="button"
					class="recent-chip"
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

<style>
	.intent-box {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		padding: 0.25rem 0 0;
	}

	.intent-row {
		position: relative;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.intent-input-wrap {
		position: relative;
		flex: 1;
	}

	.intent-input {
		width: 100%;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
		padding: 0.65rem 0.9rem 0.65rem 2.5rem;
		font-family: var(--font-body);
		font-size: 0.9rem;
		color: var(--alt-charcoal);
		transition: border-color 0.15s;
	}

	.intent-input::placeholder {
		color: var(--alt-ash);
	}

	.intent-input:focus {
		border-color: var(--alt-primary);
		outline: none;
	}

	.intent-icon {
		position: absolute;
		left: 0.85rem;
		top: 50%;
		transform: translateY(-50%);
		display: flex;
		color: var(--alt-ash);
	}

	.intent-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		align-self: stretch;
		padding: 0 0.95rem;
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-weight: 500;
		border: 1px solid var(--chip-border);
		background: var(--action-surface);
		color: var(--interactive-text);
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.intent-btn:hover {
		background: var(--action-surface-hover);
		color: var(--interactive-text-hover);
	}

	.intent-btn--clear {
		background: transparent;
		border-color: var(--surface-border);
		color: var(--alt-slate);
	}

	.intent-btn--clear:hover {
		background: var(--surface-hover);
		color: var(--alt-charcoal);
	}

	/* ── Recent queries ── */
	.recent-row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.35rem;
	}

	.recent-label {
		font-family: var(--font-mono);
		font-size: 0.64rem;
		font-weight: 600;
		letter-spacing: 0.12em;
		color: var(--alt-ash);
		margin-right: 0.25rem;
	}

	.recent-chip {
		border: 1px solid var(--surface-border);
		background: transparent;
		padding: 0.15rem 0.6rem;
		font-family: var(--font-body);
		font-size: 0.78rem;
		color: var(--alt-slate);
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s;
	}

	.recent-chip:hover {
		border-color: var(--interactive-text);
		color: var(--interactive-text);
	}
</style>

<script lang="ts">
interface Props {
	active: boolean;
	searching: boolean;
	onSearch: (query: string) => void;
	onClear: () => void;
}

const { active, searching, onSearch, onClear }: Props = $props();

let query = $state("");

// Pull-only (D25): the only path to a fetch is this explicit submit handler
// (Enter or the search button both trigger form submit). Never wired to a
// keystroke or an $effect. An empty/whitespace query is a no-op.
function handleSubmit(e: SubmitEvent) {
	e.preventDefault();
	const trimmed = query.trim();
	if (!trimmed) return;
	onSearch(trimmed);
}

function handleClear() {
	query = "";
	onClear();
}
</script>

<form class="trail-search" onsubmit={handleSubmit}>
	<button
		type="submit"
		class="trail-search-submit"
		aria-label="Search trail"
		disabled={searching}
	>
		<svg class="ic" aria-hidden="true" viewBox="0 0 24 24">
			<circle cx="11" cy="11" r="7" />
			<path d="m21 21-4.3-4.3" />
		</svg>
	</button>
	<input
		type="text"
		class="trail-search-input"
		data-testid="trail-search"
		placeholder="Search what you've read…"
		aria-label="Search your trail"
		bind:value={query}
	/>
	{#if active}
		<button
			type="button"
			class="trail-search-clear"
			data-testid="trail-search-clear"
			onclick={handleClear}
		>
			Clear
		</button>
	{/if}
</form>

<style>
	.trail-search {
		position: relative;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 1.3rem 0 0;
		max-width: 880px;
	}
	.trail-search-submit {
		position: absolute;
		left: 0.5rem;
		top: 50%;
		transform: translateY(-50%);
		display: flex;
		align-items: center;
		justify-content: center;
		border: none;
		background: none;
		color: var(--alt-ash, #999);
		cursor: pointer;
		padding: 0.25rem;
	}
	.trail-search-submit:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.ic {
		width: 1rem;
		height: 1rem;
		stroke: currentColor;
		fill: none;
		stroke-width: 2;
		stroke-linecap: round;
		stroke-linejoin: round;
		flex: none;
	}
	.trail-search-input {
		flex: 1;
		font-family: var(--font-body);
		font-size: 0.9rem;
		color: var(--alt-charcoal, #1a1a1a);
		background: var(--surface-bg, #faf9f7);
		border: 1px solid var(--surface-border, #c8c8c8);
		padding: 0.65rem 0.9rem 0.65rem 2.5rem;
	}
	.trail-search-input::placeholder {
		color: var(--alt-ash, #999);
	}
	.trail-search-input:focus {
		outline: none;
		border-color: var(--alt-primary, #2f4f4f);
	}
	.trail-search-clear {
		flex: none;
		border: 1px solid var(--chip-border, #d0c8bb);
		background: var(--action-surface, #ebe8e1);
		color: var(--interactive-text, #2f4f4f);
		font-family: var(--font-body);
		font-size: 0.82rem;
		padding: 0.5rem 0.85rem;
		cursor: pointer;
	}
	.trail-search-clear:hover {
		background: var(--action-surface-hover, #e0dbd2);
	}
</style>

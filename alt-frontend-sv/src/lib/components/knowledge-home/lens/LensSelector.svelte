<script lang="ts">
import { Filter, Plus, X } from "@lucide/svelte";
import type { LensData } from "#lib/connect/knowledge_home.js";

interface Props {
	lenses: LensData[];
	activeLensId: string | null;
	matchCount?: number | null;
	onSelect: (lensId: string | null) => void;
	onCreateClick: () => void;
	onDelete?: (lensId: string) => void;
}

const {
	lenses,
	activeLensId,
	matchCount = null,
	onSelect,
	onCreateClick,
	onDelete,
}: Props = $props();

const activeLens = $derived(
	activeLensId ? lenses.find((l) => l.lensId === activeLensId) : null,
);
const activeSummary = $derived.by(() => {
	if (!activeLens?.currentVersion) return [];
	const parts: string[] = [];
	if (matchCount != null) {
		parts.push(`${matchCount} matches`);
	}
	if (activeLens.currentVersion.queryText) {
		parts.push(`Search: "${activeLens.currentVersion.queryText}"`);
	}
	if (activeLens.currentVersion.tagIds.length) {
		parts.push(`Tags: ${activeLens.currentVersion.tagIds.join(", ")}`);
	}
	if (activeLens.currentVersion.sourceIds.length) {
		const count = activeLens.currentVersion.sourceIds.length;
		parts.push(`Sources: ${count} selected`);
	}
	if (activeLens.currentVersion.timeWindow) {
		parts.push(`Window: ${activeLens.currentVersion.timeWindow}`);
	}
	if (parts.length === 0) {
		parts.push("Server-side filtered view");
	}
	return parts;
});
</script>

<div class="lens-bar">
	<div class="lens-pills">
		<button
			class="lens-pill {activeLensId === null ? 'lens-pill--active' : ''}"
			onclick={() => onSelect(null)}
		>
			<Filter class="h-3.5 w-3.5" />
			All
		</button>

		{#each lenses as lens (lens.lensId)}
			<div class="lens-pill-group {activeLensId === lens.lensId ? 'lens-pill-group--active' : ''}">
				<button
					class="lens-pill {activeLensId === lens.lensId ? 'lens-pill--active' : ''}"
					onclick={() => onSelect(lens.lensId)}
				>
					{lens.name}
				</button>
				{#if onDelete}
					<button
						type="button"
						class="lens-pill-delete"
						aria-label="Delete lens {lens.name}"
						title="Delete lens {lens.name}"
						onclick={(e) => {
							e.stopPropagation();
							onDelete(lens.lensId);
						}}
					>
						<X class="h-3 w-3" />
					</button>
				{/if}
			</div>
		{/each}

		<button class="lens-pill lens-pill--create" onclick={onCreateClick}>
			<Plus class="h-3.5 w-3.5" />
			Save current view
		</button>
	</div>

	{#if activeLens}
		<div class="lens-summary">
			<div class="min-w-0">
				<p class="lens-summary-name">Active lens: {activeLens.name}</p>
				<p class="lens-summary-detail">{activeSummary.join(" · ")}</p>
			</div>
			<div class="flex items-center gap-2 flex-shrink-0">
				{#if onDelete}
					<button
						type="button"
						class="lens-delete"
						onclick={() => onDelete(activeLens.lensId)}
					>
						Delete
					</button>
				{/if}
				<button class="lens-clear" onclick={() => onSelect(null)}>
					Clear
				</button>
			</div>
		</div>
	{/if}
</div>

<style>
	.lens-bar {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.lens-pills {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.lens-pill-group {
		display: inline-flex;
		align-items: stretch;
	}

	.lens-pill {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 0.375rem 0.75rem;
		font-family: var(--font-body);
		font-size: 0.875rem;
		font-weight: 500;
		border: 1px solid var(--surface-border);
		background: transparent;
		color: var(--alt-slate);
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s;
	}

	.lens-pill:hover {
		border-color: var(--alt-slate);
	}

	.lens-pill--active {
		border-color: var(--alt-primary);
		color: var(--alt-primary);
		background: color-mix(in srgb, var(--alt-primary) 15%, transparent);
	}

	.lens-pill-delete {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 0 0.4rem;
		border: 1px solid var(--surface-border);
		border-left: none;
		background: transparent;
		color: var(--alt-ash);
		cursor: pointer;
		transition: color 0.15s, border-color 0.15s;
	}

	.lens-pill-group--active .lens-pill-delete {
		border-color: var(--alt-primary);
		background: color-mix(in srgb, var(--alt-primary) 15%, transparent);
	}

	.lens-pill-delete:hover {
		color: var(--alt-error, #b91c1c);
	}

	.lens-pill--create {
		border-style: dashed;
		color: var(--alt-ash);
	}

	.lens-pill--create:hover {
		color: var(--alt-slate);
		border-color: var(--alt-slate);
	}

	.lens-summary {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-2);
		padding: 0.75rem 1rem;
	}

	.lens-summary-name {
		font-family: var(--font-body);
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--alt-charcoal);
	}

	.lens-summary-detail {
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--alt-slate);
	}

	.lens-clear,
	.lens-delete {
		flex-shrink: 0;
		border: 1px solid var(--surface-border);
		padding: 0.25rem 0.75rem;
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--alt-charcoal);
		background: transparent;
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s;
	}

	.lens-clear:hover {
		border-color: var(--alt-primary);
		color: var(--alt-primary);
	}

	.lens-delete:hover {
		border-color: var(--alt-error, #b91c1c);
		color: var(--alt-error, #b91c1c);
	}
</style>

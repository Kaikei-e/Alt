<script lang="ts">
import { Filter, Plus } from "@lucide/svelte";
import type { LensData } from "$lib/connect/knowledge_home";

interface Props {
	lenses: LensData[];
	activeLensId: string | null;
	onSelect: (lensId: string | null) => void;
	onCreateClick: () => void;
}

const { lenses, activeLensId, onSelect, onCreateClick }: Props = $props();

const activeLens = $derived(
	activeLensId ? lenses.find((l) => l.lensId === activeLensId) : null,
);
</script>

<div class="flex items-center gap-2 flex-wrap">
	<button
		class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-full border transition-colors
			{activeLensId === null
				? 'border-[var(--accent-primary)] text-[var(--accent-primary)] bg-[var(--accent-primary)]/10'
				: 'border-[var(--surface-border)] text-[var(--text-secondary)] hover:border-[var(--text-secondary)]'}"
		onclick={() => onSelect(null)}
	>
		<Filter class="h-3.5 w-3.5" />
		All
	</button>

	{#each lenses as lens (lens.lensId)}
		<button
			class="inline-flex items-center gap-1 px-3 py-1.5 text-sm rounded-full border transition-colors
				{activeLensId === lens.lensId
					? 'border-[var(--accent-primary)] text-[var(--accent-primary)] bg-[var(--accent-primary)]/10'
					: 'border-[var(--surface-border)] text-[var(--text-secondary)] hover:border-[var(--text-secondary)]'}"
			onclick={() => onSelect(lens.lensId)}
		>
			{lens.name}
		</button>
	{/each}

	<button
		class="inline-flex items-center gap-1 px-2 py-1.5 text-sm rounded-full border border-dashed border-[var(--surface-border)] text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] hover:border-[var(--text-secondary)] transition-colors"
		onclick={onCreateClick}
	>
		<Plus class="h-3.5 w-3.5" />
		Save view
	</button>
</div>

<script lang="ts">
import { Sparkles } from "@lucide/svelte";

interface Props {
	activeLensName?: string | null;
	onClearLens?: () => void;
}

const { activeLensName = null, onClearLens }: Props = $props();
</script>

<div
	class="flex flex-col items-center justify-center py-16 text-center"
>
	<Sparkles class="h-10 w-10 text-[var(--text-secondary)] mb-4" />
	<h3 class="text-base font-medium text-[var(--text-primary)] mb-1">
		{activeLensName ? `No matches in ${activeLensName}` : "Your knowledge is warming up"}
	</h3>
	<p class="text-sm text-[var(--text-secondary)] max-w-sm">
		{#if activeLensName}
			This lens does not match any articles right now. Clear the lens or adjust its filters.
		{:else}
			New articles and insights will appear here as they arrive and get processed.
		{/if}
	</p>
	{#if activeLensName && onClearLens}
		<button
			class="mt-4 inline-flex items-center rounded-full border border-[var(--surface-border)] px-4 py-2 text-sm text-[var(--text-primary)] transition-colors hover:border-[var(--accent-primary)] hover:text-[var(--accent-primary)]"
			onclick={onClearLens}
		>
			Clear lens
		</button>
	{/if}
</div>

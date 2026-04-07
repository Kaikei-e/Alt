<script lang="ts">
import { Newspaper } from "@lucide/svelte";

type Props = {
	requestedDate?: string | null;
};

let { requestedDate }: Props = $props();
</script>

<div class="flex flex-col items-center justify-center py-16 px-4 text-center min-h-[40vh]">
	<Newspaper class="h-12 w-12 mb-4" style="color: var(--text-secondary);" />
	<h2 class="font-['Playfair_Display'] text-xl font-bold mb-2" style="color: var(--text-primary);">
		No Morning Letter Available
	</h2>
	<p class="text-sm max-w-md" style="color: var(--text-secondary);">
		{#if requestedDate}
			No letter was generated for {requestedDate}. Try a different date or check back later.
		{:else}
			The morning briefing hasn't been generated yet. Check back soon.
		{/if}
	</p>
</div>

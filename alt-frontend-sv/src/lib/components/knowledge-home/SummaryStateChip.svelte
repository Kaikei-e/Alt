<script lang="ts">
import { Loader } from "@lucide/svelte";
import type { SummaryState } from "$lib/connect/knowledge_home";

interface Props {
	state: SummaryState;
}

const { state }: Props = $props();
</script>

{#if state === "pending"}
	<span class="summary-chip">
		<Loader class="h-3 w-3 animate-spin" />
		Summarizing
	</span>
{/if}

<style>
	.summary-chip {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		padding: 0.15rem 0.4rem;
		font-family: var(--font-body);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--accent-info-text);
		background: var(--accent-info-bg);
		border: 1px solid var(--accent-info-border);
	}
</style>

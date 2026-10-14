<script lang="ts">
import { Loader } from "@lucide/svelte";
import type { SummaryState } from "#lib/connect/knowledge_home.js";

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
		padding: 0.14rem 0.5rem;
		font-family: var(--font-mono);
		font-size: 0.62rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		line-height: 1.4;
		text-transform: uppercase;
		color: var(--accent-info-text);
		background: var(--accent-info-bg);
		border: 1px solid var(--accent-info-border);
	}
</style>

<script lang="ts">
import { Loader } from "@lucide/svelte";
import type { SummaryState } from "$lib/connect/knowledge_home";

interface Props {
	state: SummaryState;
}

const { state }: Props = $props();
</script>

{#if state === "pending"}
	<span
		class="inline-flex items-center gap-1 px-1.5 py-0.5 text-xs bg-blue-500/10 text-blue-400"
	>
		<Loader class="h-3 w-3 animate-spin" />
		Summarizing
	</span>
{/if}

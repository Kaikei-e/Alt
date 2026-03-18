<script lang="ts">
import type { SupersedeInfoData } from "$lib/connect/knowledge_home";
import { resolveSupersede } from "./supersede-display-map";

interface Props {
	info: SupersedeInfoData;
}

const { info }: Props = $props();
const display = $derived(resolveSupersede(info.state));

function formatTime(isoString: string): string {
	if (!isoString) return "";
	const date = new Date(isoString);
	if (Number.isNaN(date.getTime())) return isoString;
	return date.toLocaleString();
}
</script>

<div
	class="mt-2 p-3 rounded border text-xs space-y-2 bg-[var(--surface-bg)] border-[var(--surface-border)]"
>
	<div class="flex items-center gap-2 text-[var(--text-secondary)]">
		<span class="font-medium">{display.label}</span>
		{#if info.supersededAt}
			<span>&middot; {formatTime(info.supersededAt)}</span>
		{/if}
	</div>

	{#if info.previousSummaryExcerpt}
		<div>
			<span class="font-medium text-[var(--text-secondary)]">Previous summary:</span>
			<p class="mt-0.5 text-[var(--text-tertiary)] line-clamp-3 italic">
				{info.previousSummaryExcerpt}
			</p>
		</div>
	{/if}

	{#if info.previousTags.length > 0}
		<div>
			<span class="font-medium text-[var(--text-secondary)]">Previous tags:</span>
			<div class="flex flex-wrap gap-1 mt-0.5">
				{#each info.previousTags as tag}
					<span
						class="inline-flex items-center rounded border px-1.5 py-0.5 text-xs bg-[var(--chip-bg)] border-[var(--chip-border)] text-[var(--chip-text)] opacity-60 line-through"
					>
						{tag}
					</span>
				{/each}
			</div>
		</div>
	{/if}

	{#if info.previousWhyCodes.length > 0}
		<div>
			<span class="font-medium text-[var(--text-secondary)]">Previous reasons:</span>
			<div class="flex flex-wrap gap-1 mt-0.5">
				{#each info.previousWhyCodes as code}
					<span
						class="inline-flex items-center rounded border px-1.5 py-0.5 text-xs bg-[var(--chip-bg)] border-[var(--chip-border)] text-[var(--chip-text)] opacity-60"
					>
						{code}
					</span>
				{/each}
			</div>
		</div>
	{/if}
</div>

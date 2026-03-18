<script lang="ts">
import type { ReprojectDiffSummaryData } from "$lib/connect/knowledge_home_admin";
import { ArrowRight } from "@lucide/svelte";

let { diff }: { diff: ReprojectDiffSummaryData | null } = $props();

const formatNumber = (n: number) => n.toLocaleString();
const formatScore = (n: number) => n.toFixed(3);

const deltaColor = (from: number, to: number) => {
	if (to > from) return "var(--accent-green, #22c55e)";
	if (to < from) return "var(--accent-red, #ef4444)";
	return "var(--text-secondary)";
};
</script>

<div class="flex flex-col gap-3">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Diff Summary
	</h3>

	{#if !diff}
		<p class="text-xs" style="color: var(--text-secondary);">
			Select a completed run and click Compare to view differences.
		</p>
	{:else}
		<div
			class="grid grid-cols-1 gap-4 rounded-lg border-2 p-4 sm:grid-cols-2"
			style="background: var(--surface-bg); border-color: var(--border-primary);"
		>
			<!-- Item Count -->
			<div class="flex flex-col gap-1">
				<span class="text-xs font-medium" style="color: var(--text-secondary);">
					Item Count
				</span>
				<div class="flex items-center gap-2">
					<span class="text-sm font-mono" style="color: var(--text-primary);">
						{formatNumber(diff.fromItemCount)}
					</span>
					<ArrowRight size={14} style="color: var(--text-secondary);" />
					<span
						class="text-sm font-mono font-bold"
						style="color: {deltaColor(diff.fromItemCount, diff.toItemCount)};"
					>
						{formatNumber(diff.toItemCount)}
					</span>
				</div>
			</div>

			<!-- Empty Count -->
			<div class="flex flex-col gap-1">
				<span class="text-xs font-medium" style="color: var(--text-secondary);">
					Empty Count
				</span>
				<div class="flex items-center gap-2">
					<span class="text-sm font-mono" style="color: var(--text-primary);">
						{formatNumber(diff.fromEmptyCount)}
					</span>
					<ArrowRight size={14} style="color: var(--text-secondary);" />
					<span
						class="text-sm font-mono font-bold"
						style="color: {deltaColor(diff.toEmptyCount, diff.fromEmptyCount)};"
					>
						{formatNumber(diff.toEmptyCount)}
					</span>
				</div>
			</div>

			<!-- Avg Score -->
			<div class="flex flex-col gap-1">
				<span class="text-xs font-medium" style="color: var(--text-secondary);">
					Average Score
				</span>
				<div class="flex items-center gap-2">
					<span class="text-sm font-mono" style="color: var(--text-primary);">
						{formatScore(diff.fromAvgScore)}
					</span>
					<ArrowRight size={14} style="color: var(--text-secondary);" />
					<span
						class="text-sm font-mono font-bold"
						style="color: {deltaColor(diff.fromAvgScore, diff.toAvgScore)};"
					>
						{formatScore(diff.toAvgScore)}
					</span>
				</div>
			</div>

			<!-- Why Distribution -->
			<div class="flex flex-col gap-1 sm:col-span-2">
				<span class="text-xs font-medium" style="color: var(--text-secondary);">
					Why Distribution
				</span>
				<div class="grid grid-cols-2 gap-3">
					<div class="flex flex-col gap-1">
						<span class="text-xs" style="color: var(--text-secondary);">From</span>
						<pre
							class="overflow-x-auto rounded border p-2 text-xs"
							style="background: var(--surface-bg); border-color: var(--surface-border, #d1d5db); color: var(--text-primary);"
						>{diff.fromWhyDistribution || "--"}</pre>
					</div>
					<div class="flex flex-col gap-1">
						<span class="text-xs" style="color: var(--text-secondary);">To</span>
						<pre
							class="overflow-x-auto rounded border p-2 text-xs"
							style="background: var(--surface-bg); border-color: var(--surface-border, #d1d5db); color: var(--text-primary);"
						>{diff.toWhyDistribution || "--"}</pre>
					</div>
				</div>
			</div>
		</div>
	{/if}
</div>

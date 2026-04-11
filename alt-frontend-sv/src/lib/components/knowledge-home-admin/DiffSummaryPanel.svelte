<script lang="ts">
import type { ReprojectDiffSummaryData } from "$lib/connect/knowledge_home_admin";
import { ArrowRight } from "@lucide/svelte";

let { diff }: { diff: ReprojectDiffSummaryData | null } = $props();

const formatNumber = (n: number) => n.toLocaleString();
const formatScore = (n: number) => n.toFixed(3);

const deltaColor = (from: number, to: number) => {
	if (to > from) return "var(--alt-sage)";
	if (to < from) return "var(--alt-terracotta)";
	return "var(--alt-ash)";
};
</script>

<div class="panel" data-role="diff-summary">
	<h3 class="section-heading">Diff Summary</h3>
	<div class="heading-rule"></div>

	{#if !diff}
		<p class="empty-text">
			Select a completed run and click Compare to view differences.
		</p>
	{:else}
		<div class="diff-grid">
			<div class="diff-item">
				<span class="diff-label">Item Count</span>
				<div class="diff-values">
					<span class="diff-from">{formatNumber(diff.fromItemCount)}</span>
					<ArrowRight size={14} style="color: var(--alt-ash);" />
					<span class="diff-to" style="color: {deltaColor(diff.fromItemCount, diff.toItemCount)};">
						{formatNumber(diff.toItemCount)}
					</span>
				</div>
			</div>

			<div class="diff-item">
				<span class="diff-label">Empty Count</span>
				<div class="diff-values">
					<span class="diff-from">{formatNumber(diff.fromEmptyCount)}</span>
					<ArrowRight size={14} style="color: var(--alt-ash);" />
					<span class="diff-to" style="color: {deltaColor(diff.toEmptyCount, diff.fromEmptyCount)};">
						{formatNumber(diff.toEmptyCount)}
					</span>
				</div>
			</div>

			<div class="diff-item">
				<span class="diff-label">Average Score</span>
				<div class="diff-values">
					<span class="diff-from">{formatScore(diff.fromAvgScore)}</span>
					<ArrowRight size={14} style="color: var(--alt-ash);" />
					<span class="diff-to" style="color: {deltaColor(diff.fromAvgScore, diff.toAvgScore)};">
						{formatScore(diff.toAvgScore)}
					</span>
				</div>
			</div>

			<div class="diff-item wide">
				<span class="diff-label">Why Distribution</span>
				<div class="diff-dist">
					<div class="diff-dist-col">
						<span class="diff-dist-heading">From</span>
						<pre class="diff-pre">{diff.fromWhyDistribution || "--"}</pre>
					</div>
					<div class="diff-dist-col">
						<span class="diff-dist-heading">To</span>
						<pre class="diff-pre">{diff.toWhyDistribution || "--"}</pre>
					</div>
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.section-heading {
		font-family: var(--font-display);
		font-size: 1.05rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.heading-rule {
		height: 1px;
		background: var(--surface-border);
		margin-bottom: 0.25rem;
	}

	.empty-text {
		font-family: var(--font-display);
		font-size: 0.8rem;
		font-style: italic;
		color: var(--alt-ash);
	}

	.diff-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 1rem;
		padding: 1rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
	}

	.diff-item {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	.diff-item.wide {
		grid-column: 1 / -1;
	}

	.diff-label {
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.diff-values {
		display: flex;
		align-items: center;
		gap: 0.35rem;
	}

	.diff-from {
		font-family: var(--font-mono);
		font-size: 0.8rem;
		color: var(--alt-charcoal);
	}

	.diff-to {
		font-family: var(--font-mono);
		font-size: 0.8rem;
		font-weight: 700;
	}

	.diff-dist {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0.75rem;
	}

	.diff-dist-col {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.diff-dist-heading {
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.diff-pre {
		overflow-x: auto;
		padding: 0.5rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-2);
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-charcoal);
		margin: 0;
		white-space: pre-wrap;
	}

	@media (max-width: 640px) {
		.diff-grid {
			grid-template-columns: 1fr;
		}
	}
</style>

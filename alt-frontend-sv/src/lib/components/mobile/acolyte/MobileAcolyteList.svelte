<script lang="ts">
import { goto } from "$app/navigation";
import type { AcolyteReportSummary } from "$lib/connect/acolyte";
import MobileAcolyteReportCard from "./MobileAcolyteReportCard.svelte";

interface Props {
	reports: AcolyteReportSummary[];
	loading: boolean;
	error: string | null;
}

const { reports, loading, error }: Props = $props();

const dateStr = new Date().toLocaleDateString("en-US", {
	weekday: "long",
	year: "numeric",
	month: "long",
	day: "numeric",
});
</script>

<div class="min-h-screen pb-[calc(2.75rem+env(safe-area-inset-bottom,0px))]">
	<!-- Masthead -->
	<header class="text-center px-4 pt-5 pb-3">
		<div class="h-[2px] mx-auto max-w-[720px]" style="background: var(--alt-charcoal, #1a1a1a);"></div>
		<div class="py-3">
			<span
				class="font-[var(--font-body)] text-[0.65rem] uppercase tracking-[0.14em] text-[var(--alt-ash,#999)]"
			>
				{dateStr}
			</span>
			<h1
				class="font-[var(--font-display)] text-[1.8rem] font-black tracking-tight leading-none mt-0.5 mb-0.5 text-[var(--alt-charcoal,#1a1a1a)]"
			>
				Acolyte
			</h1>
			<p
				class="font-[var(--font-body)] text-[0.75rem] italic text-[var(--alt-slate,#666)] m-0"
			>
				Intelligence Briefings &amp; Analytical Reports
			</p>
		</div>
		<div class="h-[2px] mx-auto max-w-[720px]" style="background: var(--alt-charcoal, #1a1a1a);"></div>
	</header>

	<!-- Toolbar -->
	<nav class="flex items-center justify-between max-w-[720px] mx-auto px-4 py-2 border-b border-[var(--surface-border,#c8c8c8)]">
		<span class="font-[var(--font-body)] text-[0.75rem] uppercase tracking-wider text-[var(--alt-ash,#999)]">
			{reports.length} report{reports.length !== 1 ? "s" : ""}
		</span>
		<a
			href="/acolyte/new"
			class="inline-flex items-center gap-1.5 font-[var(--font-body)] text-[0.8rem] font-semibold uppercase tracking-wide px-3 py-2 border-[1.5px] border-[var(--alt-charcoal,#1a1a1a)] text-[var(--alt-charcoal,#1a1a1a)] no-underline transition-colors duration-200 min-h-[44px] active:bg-[var(--alt-charcoal,#1a1a1a)] active:text-[var(--surface-bg,#faf9f7)]"
		>
			<span class="text-base leading-none">+</span>
			New Report
		</a>
	</nav>

	{#if error}
		<div class="max-w-[720px] mx-auto mt-3 mx-4 px-4 py-2 font-[var(--font-body)] text-[0.85rem] text-[var(--alt-terracotta,#b85450)] border-l-[3px] border-l-[var(--alt-terracotta,#b85450)]" style="background: #fef2f2;">
			{error}
		</div>
	{/if}

	{#if loading}
		<div class="flex items-center justify-center gap-3 py-12 text-[var(--alt-ash,#999)] font-[var(--font-body)] text-[0.85rem]">
			<div class="w-2 h-2 rounded-full animate-pulse" style="background: var(--alt-ash, #999);"></div>
			<span>Retrieving reports&hellip;</span>
		</div>
	{:else if reports.length === 0}
		<div class="text-center py-16 px-4 font-[var(--font-body)] text-[var(--alt-ash,#999)]">
			<div class="text-2xl mb-3 text-[var(--surface-border,#c8c8c8)]">&#9670;</div>
			<p>No reports have been composed yet.</p>
			<a
				href="/acolyte/new"
				class="inline-block mt-4 text-[0.8rem] font-semibold uppercase tracking-wide px-5 py-2 border-[1.5px] border-[var(--alt-charcoal,#1a1a1a)] text-[var(--alt-charcoal,#1a1a1a)] no-underline transition-colors duration-200 active:bg-[var(--alt-charcoal,#1a1a1a)] active:text-[var(--surface-bg,#faf9f7)]"
			>
				Create Your First Report
			</a>
		</div>
	{:else}
		<div class="max-w-[720px] mx-auto mt-3">
			{#each reports as report, i}
				<div class="card-enter" style="--stagger: {i};">
					<MobileAcolyteReportCard
						{report}
						onClick={(reportId) => goto(`/acolyte/reports/${reportId}`)}
					/>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.card-enter {
		opacity: 0;
		animation: card-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 60ms);
	}
	@keyframes card-in {
		to { opacity: 1; }
	}
</style>

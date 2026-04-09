<script lang="ts">
interface Props {
	onSubmit: (title: string, reportType: string, topic: string) => void;
	error?: string | null;
	submitting?: boolean;
}

const { onSubmit, error = null, submitting = false }: Props = $props();

let title = $state("");
let reportType = $state("weekly_briefing");
let topic = $state("");

const reportTypes = [
	{
		value: "weekly_briefing",
		label: "Weekly Briefing",
		desc: "Curated intelligence summary of the past week",
	},
	{
		value: "market_analysis",
		label: "Market Analysis",
		desc: "Sector trends, sentiment, and outlook",
	},
	{
		value: "tech_review",
		label: "Technology Review",
		desc: "Emerging technology landscape and impact assessment",
	},
	{
		value: "custom",
		label: "Custom Report",
		desc: "Free-form topic with full pipeline",
	},
] as const;

function handleSubmit() {
	if (!title.trim()) return;
	onSubmit(title.trim(), reportType, topic.trim());
}
</script>

<div class="min-h-screen pb-[calc(2.75rem+env(safe-area-inset-bottom,0px))] px-4 pt-4">
	<!-- Back link -->
	<nav class="mb-4">
		<a
			href="/acolyte"
			class="inline-flex items-center min-h-[44px] font-[var(--font-body)] text-[0.7rem] uppercase tracking-wider text-[var(--alt-ash,#999)] no-underline active:text-[var(--alt-charcoal,#1a1a1a)]"
		>
			&larr; All Reports
		</a>
	</nav>

	<!-- Header -->
	<header class="mb-6">
		<div class="h-[2px] mb-3" style="background: var(--alt-charcoal, #1a1a1a);"></div>
		<h1 class="font-[var(--font-display)] text-[1.3rem] font-extrabold m-0 text-[var(--alt-charcoal,#1a1a1a)]">
			Compose New Report
		</h1>
		<p class="font-[var(--font-body)] text-[0.8rem] italic text-[var(--alt-slate,#666)] mt-1 mb-0">
			Define the scope and the pipeline will handle the rest.
		</p>
	</header>

	{#if error}
		<div class="mb-4 px-4 py-2 font-[var(--font-body)] text-[0.85rem] text-[var(--alt-terracotta,#b85450)] border-l-[3px] border-l-[var(--alt-terracotta,#b85450)]" style="background: #fef2f2;">
			{error}
		</div>
	{/if}

	<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="flex flex-col gap-6">
		<!-- Title -->
		<div class="flex flex-col gap-1.5">
			<label
				for="aco-title"
				class="font-[var(--font-body)] text-[0.7rem] font-semibold uppercase tracking-widest text-[var(--alt-slate,#666)]"
			>
				Title
			</label>
			<input
				id="aco-title"
				type="text"
				bind:value={title}
				placeholder="Weekly AI Briefing — April 2026"
				autocomplete="off"
				class="font-[var(--font-body)] text-base px-3 py-3 border border-[var(--surface-border,#c8c8c8)] bg-transparent text-[var(--alt-charcoal,#1a1a1a)] outline-none min-h-[44px] transition-colors duration-150 focus:border-[var(--alt-charcoal,#1a1a1a)]"
			/>
		</div>

		<!-- Report Type -->
		<fieldset class="m-0 p-0 border-none">
			<legend class="font-[var(--font-body)] text-[0.7rem] font-semibold uppercase tracking-widest text-[var(--alt-slate,#666)] mb-2">
				Report Type
			</legend>
			<div class="flex flex-col gap-2">
				{#each reportTypes as rt}
					<label
						class="flex flex-col gap-0.5 px-3 py-3 border border-[var(--surface-border,#c8c8c8)] cursor-pointer transition-colors duration-150 min-h-[48px] active:bg-[var(--surface-hover,#f3f1ed)]"
						class:border-[var(--alt-charcoal,#1a1a1a)]={reportType === rt.value}
						class:bg-[var(--surface-2,#f5f4f1)]={reportType === rt.value}
					>
						<input
							type="radio"
							name="report-type"
							value={rt.value}
							bind:group={reportType}
							class="absolute opacity-0 w-0 h-0"
						/>
						<span class="font-[var(--font-display)] text-[0.85rem] font-bold text-[var(--alt-charcoal,#1a1a1a)]">
							{rt.label}
						</span>
						<span class="font-[var(--font-body)] text-[0.7rem] text-[var(--alt-ash,#999)] leading-snug">
							{rt.desc}
						</span>
					</label>
				{/each}
			</div>
		</fieldset>

		<!-- Topic -->
		<div class="flex flex-col gap-1.5">
			<label
				for="aco-topic"
				class="font-[var(--font-body)] text-[0.7rem] font-semibold uppercase tracking-widest text-[var(--alt-slate,#666)]"
			>
				Topic / Scope
			</label>
			<textarea
				id="aco-topic"
				bind:value={topic}
				rows="3"
				placeholder="AI market trends, LLM developments, semiconductor supply chain..."
				class="font-[var(--font-body)] text-base px-3 py-3 border border-[var(--surface-border,#c8c8c8)] bg-transparent text-[var(--alt-charcoal,#1a1a1a)] outline-none resize-y transition-colors duration-150 focus:border-[var(--alt-charcoal,#1a1a1a)]"
			></textarea>
			<span class="font-[var(--font-body)] text-[0.75rem] text-[var(--alt-ash,#999)] italic">
				Describe the subject matter. The planner will generate a structured outline from this.
			</span>
		</div>

		<!-- Actions -->
		<div class="flex flex-col gap-3 pt-5 border-t border-[var(--surface-border,#c8c8c8)]">
			<button
				type="submit"
				disabled={submitting || !title.trim()}
				class="inline-flex items-center justify-center gap-2 w-full font-[var(--font-body)] text-[0.8rem] font-semibold tracking-wide px-5 py-3 border-[1.5px] border-[var(--alt-charcoal,#1a1a1a)] min-h-[44px] cursor-pointer transition-opacity duration-150 disabled:opacity-40 disabled:cursor-not-allowed"
				style="background: var(--alt-charcoal, #1a1a1a); color: var(--surface-bg, #faf9f7);"
			>
				{#if submitting}
					<span class="inline-block w-3 h-3 border-[1.5px] border-[var(--surface-bg,#faf9f7)] border-t-transparent rounded-full animate-spin"></span>
					Creating&hellip;
				{:else}
					Create Report
				{/if}
			</button>
			<a
				href="/acolyte"
				class="text-center font-[var(--font-body)] text-[0.8rem] py-2 text-[var(--alt-slate,#666)] no-underline transition-colors duration-150 active:text-[var(--alt-charcoal,#1a1a1a)]"
			>
				Cancel
			</a>
		</div>
	</form>
</div>

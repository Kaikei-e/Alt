<script lang="ts">
import { goto } from "$app/navigation";
import { onMount } from "svelte";
import { createReport } from "$lib/connect/acolyte";
import { useViewport } from "$lib/stores/viewport.svelte";
import MobileAcolyteNew from "$lib/components/mobile/acolyte/MobileAcolyteNew.svelte";

const { isDesktop } = useViewport();

let title = $state("");
let reportType = $state("weekly_briefing");
let topic = $state("");
let submitting = $state(false);
let error = $state<string | null>(null);
let revealed = $state(false);

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

let selectedTypeDesc = $derived(
	reportTypes.find((t) => t.value === reportType)?.desc ?? "",
);

async function handleSubmit() {
	if (!title.trim()) {
		error = "A title is required to proceed.";
		return;
	}
	try {
		submitting = true;
		error = null;
		const scope: Record<string, string> = {};
		if (topic.trim()) scope.topic = topic.trim();
		const result = await createReport(title.trim(), reportType, scope);
		goto(`/acolyte/reports/${result.reportId}`);
	} catch (e) {
		error = e instanceof Error ? e.message : "Failed to create report";
	} finally {
		submitting = false;
	}
}

async function handleMobileSubmit(
	titleVal: string,
	reportTypeVal: string,
	topicVal: string,
) {
	try {
		submitting = true;
		error = null;
		const scope: Record<string, string> = {};
		if (topicVal) scope.topic = topicVal;
		const result = await createReport(titleVal, reportTypeVal, scope);
		goto(`/acolyte/reports/${result.reportId}`);
	} catch (e) {
		error = e instanceof Error ? e.message : "Failed to create report";
	} finally {
		submitting = false;
	}
}

onMount(() => {
	requestAnimationFrame(() => {
		revealed = true;
	});
});
</script>

{#if !isDesktop}
<MobileAcolyteNew
	onSubmit={handleMobileSubmit}
	{error}
	{submitting}
/>
{:else}
<div class="aco-new" class:revealed>
	<!-- Breadcrumb -->
	<nav class="aco-breadcrumb">
		<a href="/acolyte">&larr; All Reports</a>
	</nav>

	<header class="new-header">
		<div class="header-rule"></div>
		<h1>Compose New Report</h1>
		<p class="header-sub">Define the scope and the pipeline will handle the rest.</p>
	</header>

	{#if error}
		<div class="aco-error">{error}</div>
	{/if}

	<form class="new-form" onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}>
		<!-- Title -->
		<div class="field">
			<label for="aco-title">Title</label>
			<input
				id="aco-title"
				type="text"
				bind:value={title}
				placeholder="Weekly AI Briefing — April 2026"
				autocomplete="off"
			/>
		</div>

		<!-- Type selector as radio cards -->
		<fieldset class="field field-type">
			<legend>Report Type</legend>
			<div class="type-grid">
				{#each reportTypes as rt}
					<label class="type-card" class:selected={reportType === rt.value}>
						<input type="radio" name="report-type" value={rt.value} bind:group={reportType} />
						<span class="type-label">{rt.label}</span>
						<span class="type-desc">{rt.desc}</span>
					</label>
				{/each}
			</div>
		</fieldset>

		<!-- Topic -->
		<div class="field">
			<label for="aco-topic">Topic / Scope</label>
			<textarea
				id="aco-topic"
				bind:value={topic}
				rows="3"
				placeholder="AI market trends, LLM developments, semiconductor supply chain..."
			></textarea>
			<span class="field-hint">
				Describe the subject matter. The planner will generate a structured outline from this.
			</span>
		</div>

		<!-- Actions -->
		<div class="form-actions">
			<a href="/acolyte" class="btn-cancel">Cancel</a>
			<button type="submit" class="btn-submit" disabled={submitting || !title.trim()}>
				{#if submitting}
					<span class="submit-spinner"></span> Creating&hellip;
				{:else}
					Create Report
				{/if}
			</button>
		</div>
	</form>
</div>
{/if}

<style>
	.aco-new { max-width: 600px; margin: 0 auto; padding: 1.5rem 1rem 3rem; opacity: 0; transform: translateY(6px); transition: opacity 0.35s ease, transform 0.35s ease; }
	.aco-new.revealed { opacity: 1; transform: translateY(0); }

	.aco-breadcrumb {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem; letter-spacing: 0.08em; text-transform: uppercase;
		margin-bottom: 1rem;
	}
	.aco-breadcrumb a { color: var(--alt-ash, #999); text-decoration: none; transition: color 0.15s; }
	.aco-breadcrumb a:hover { color: var(--alt-charcoal, #1a1a1a); }

	.new-header { margin-bottom: 1.75rem; }
	.header-rule { height: 2px; background: var(--alt-charcoal, #1a1a1a); margin-bottom: 0.75rem; }
	.new-header h1 {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.5rem; font-weight: 800; margin: 0;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.header-sub {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem; font-style: italic; color: var(--alt-slate, #666); margin: 0.25rem 0 0;
	}

	.new-form { display: flex; flex-direction: column; gap: 1.5rem; }

	.field { display: flex; flex-direction: column; gap: 0.35rem; }
	.field label, .field legend {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem; font-weight: 600; letter-spacing: 0.1em; text-transform: uppercase;
		color: var(--alt-slate, #666); padding: 0; border: none;
	}
	.field input[type="text"], .field textarea {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.95rem; padding: 0.6rem 0.75rem; border: 1px solid var(--surface-border, #c8c8c8);
		background: transparent; color: var(--alt-charcoal, #1a1a1a);
		outline: none; transition: border-color 0.15s; resize: vertical;
	}
	.field input:focus, .field textarea:focus { border-color: var(--alt-charcoal, #1a1a1a); }
	.field-hint {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem; color: var(--alt-ash, #999); font-style: italic;
	}

	/* Type radio cards */
	.field-type { margin: 0; }
	.type-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem; }
	.type-card {
		display: flex; flex-direction: column; gap: 0.15rem;
		padding: 0.65rem 0.75rem; border: 1px solid var(--surface-border, #c8c8c8);
		cursor: pointer; transition: border-color 0.15s, background-color 0.15s;
	}
	.type-card:hover { background: var(--surface-hover, #f3f1ed); }
	.type-card.selected { border-color: var(--alt-charcoal, #1a1a1a); background: var(--surface-2, #f5f4f1); }
	.type-card input { position: absolute; opacity: 0; width: 0; height: 0; }
	.type-label {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 0.85rem; font-weight: 700; color: var(--alt-charcoal, #1a1a1a);
	}
	.type-desc {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem; color: var(--alt-ash, #999); line-height: 1.35;
	}

	.form-actions {
		display: flex; justify-content: flex-end; gap: 0.75rem;
		padding-top: 1.25rem; border-top: 1px solid var(--surface-border, #c8c8c8);
	}
	.btn-cancel {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem; padding: 0.45rem 1rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		color: var(--alt-slate, #666); text-decoration: none;
		transition: border-color 0.15s, color 0.15s;
	}
	.btn-cancel:hover { border-color: var(--alt-charcoal, #1a1a1a); color: var(--alt-charcoal, #1a1a1a); }

	.btn-submit {
		display: inline-flex; align-items: center; gap: 0.4rem;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem; font-weight: 600; letter-spacing: 0.03em;
		padding: 0.45rem 1.25rem; border: 1.5px solid var(--alt-charcoal, #1a1a1a);
		background: var(--alt-charcoal, #1a1a1a); color: var(--surface-bg, #faf9f7);
		cursor: pointer; transition: opacity 0.15s;
	}
	.btn-submit:hover { opacity: 0.88; }
	.btn-submit:disabled { opacity: 0.4; cursor: not-allowed; }

	.submit-spinner {
		display: inline-block; width: 12px; height: 12px;
		border: 1.5px solid var(--surface-bg, #faf9f7);
		border-top-color: transparent; border-radius: 50%;
		animation: spin 0.6s linear infinite;
	}
	@keyframes spin { to { transform: rotate(360deg); } }

	.aco-error {
		padding: 0.6rem 1rem; font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.85rem; color: var(--alt-terracotta, #b85450);
		border-left: 3px solid var(--alt-terracotta, #b85450); background: #fef2f2;
	}

	@media (max-width: 480px) {
		.type-grid { grid-template-columns: 1fr; }
	}
</style>

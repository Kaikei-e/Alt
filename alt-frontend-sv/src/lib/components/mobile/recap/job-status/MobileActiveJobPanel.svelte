<script lang="ts">
import type { ActiveJobInfo } from "$lib/schema/dashboard";
import MobilePipelineProgress from "./MobilePipelineProgress.svelte";
import MobileGenreProgressGrid from "./MobileGenreProgressGrid.svelte";
import StatusGlyph from "$lib/components/recap/job-status/StatusGlyph.svelte";

interface Props {
	job: ActiveJobInfo | null;
}

let { job }: Props = $props();

let isExpanded = $state(true);

const startedAt = $derived(
	job
		? new Date(job.kicked_at).toLocaleTimeString("ja-JP", {
				hour: "2-digit",
				minute: "2-digit",
			})
		: "",
);

const elapsedTime = $derived.by(() => {
	if (!job) return "";
	const start = new Date(job.kicked_at).getTime();
	const now = Date.now();
	const secs = Math.max(0, Math.floor((now - start) / 1000));
	if (secs < 60) return `${secs}s`;
	const mins = Math.floor(secs / 60);
	return `${mins}m ${secs % 60}s`;
});

$effect(() => {
	if (job) {
		isExpanded = true;
	}
});
</script>

<section class="active-panel" data-testid="mobile-active-job-panel">
	{#if job}
		<article
			class="active-card"
			data-role="active-job"
			data-status={job.status}
		>
			<button
				type="button"
				class="head"
				onclick={() => (isExpanded = !isExpanded)}
				data-testid="active-job-collapse-toggle"
				aria-expanded={isExpanded}
			>
				<div class="head-left">
					<span class="kicker">Active job</span>
					<span class="job-id">{job.job_id.slice(0, 12)}…</span>
				</div>
				<div class="head-right">
					<StatusGlyph
						status={job.status}
						pulse={job.status === "running"}
						includeLabel={true}
					/>
					<span class="caret" aria-hidden="true">{isExpanded ? "−" : "+"}</span>
				</div>
			</button>

			{#if isExpanded}
				<div class="body">
					<dl class="meta">
						<div class="meta-cell">
							<dt>Started</dt>
							<dd class="tabular-nums">{startedAt}</dd>
						</div>
						<div class="meta-cell">
							<dt>Elapsed</dt>
							<dd class="tabular-nums">{elapsedTime}</dd>
						</div>
					</dl>

					<MobilePipelineProgress
						currentStage={job.current_stage}
						stageIndex={job.stage_index}
						stagesCompleted={job.stages_completed}
						subStageProgress={job.sub_stage_progress}
					/>

					{#if Object.keys(job.genre_progress).length > 0}
						<MobileGenreProgressGrid genreProgress={job.genre_progress} />
					{/if}
				</div>
			{/if}
		</article>
	{:else}
		<div class="empty" data-role="active-empty">
			<p>No active job.</p>
		</div>
	{/if}
</section>

<style>
	.active-panel {
		padding: 0 1rem;
		margin-bottom: 1rem;
	}

	.active-card {
		display: flex;
		flex-direction: column;
		background: var(--surface-bg);
		border-top: 2px solid var(--alt-charcoal);
		border-bottom: 2px solid var(--alt-charcoal);
		border-left: 1px solid var(--surface-border);
		border-right: 1px solid var(--surface-border);
	}

	.head {
		all: unset;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.85rem 1rem;
		min-height: 44px;
		cursor: pointer;
	}

	.head:focus-visible {
		outline: 2px solid var(--alt-charcoal);
		outline-offset: -2px;
	}

	.head-left {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}

	.head-right {
		display: flex;
		align-items: baseline;
		gap: 0.65rem;
	}

	.kicker {
		font-family: var(--font-body);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.job-id {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-charcoal);
	}

	.caret {
		font-family: var(--font-mono);
		font-size: 1rem;
		color: var(--alt-slate);
		min-width: 1ch;
		text-align: center;
	}

	.body {
		display: flex;
		flex-direction: column;
		gap: 0.85rem;
		padding: 0.85rem 1rem;
		border-top: 1px solid var(--surface-border);
	}

	.meta {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 0;
		margin: 0;
		border-top: 1px solid var(--surface-border);
		border-bottom: 1px solid var(--surface-border);
	}

	.meta-cell {
		padding: 0.5rem 0.6rem;
		border-right: 1px solid var(--surface-border);
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}

	.meta-cell:last-child {
		border-right: none;
	}

	.meta-cell dt {
		font-family: var(--font-body);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.meta-cell dd {
		margin: 0;
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-charcoal);
	}

	.empty {
		padding: 1.25rem 1rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
		text-align: center;
	}

	.empty p {
		font-family: var(--font-body);
		font-size: 0.95rem;
		font-style: italic;
		color: var(--alt-slate);
		margin: 0;
	}
</style>

<script lang="ts">
import type { ActiveJobInfo } from "$lib/schema/dashboard";
import PipelineProgress from "./PipelineProgress.svelte";
import GenreProgressList from "./GenreProgressList.svelte";
import StatusGlyph from "$lib/components/recap/job-status/StatusGlyph.svelte";

interface Props {
	job: ActiveJobInfo;
}

let { job }: Props = $props();

const startedAt = $derived(
	new Date(job.kicked_at).toLocaleString(undefined, {
		hour: "2-digit",
		minute: "2-digit",
		second: "2-digit",
		month: "short",
		day: "numeric",
	}),
);

const elapsedTime = $derived(() => {
	const start = new Date(job.kicked_at).getTime();
	const now = Date.now();
	const secs = Math.max(0, Math.floor((now - start) / 1000));
	if (secs < 60) return `${secs}s`;
	const mins = Math.floor(secs / 60);
	return `${mins}m ${secs % 60}s`;
});

const sourceLabel = $derived(
	job.trigger_source === "user" ? "User" : "System",
);
</script>

<article
	class="active-card"
	data-role="active-job"
	data-status={job.status}
>
	<header class="card-head">
		<div class="kicker-row">
			<span class="kicker">Active job</span>
			<StatusGlyph
				status={job.status}
				pulse={job.status === "running"}
				includeLabel={true}
			/>
		</div>
		<p class="job-id" data-role="active-job-id">{job.job_id}</p>
	</header>

	<dl class="meta">
		<div class="meta-cell">
			<dt>Started</dt>
			<dd class="tabular-nums">{startedAt}</dd>
		</div>
		<div class="meta-cell">
			<dt>Elapsed</dt>
			<dd class="tabular-nums">{elapsedTime()}</dd>
		</div>
		<div class="meta-cell">
			<dt>Source</dt>
			<dd>{sourceLabel}</dd>
		</div>
		{#if job.total_articles}
			<div class="meta-cell">
				<dt>Articles</dt>
				<dd class="tabular-nums">{job.total_articles}</dd>
			</div>
		{/if}
		{#if job.user_article_count !== null}
			<div class="meta-cell">
				<dt>Your articles</dt>
				<dd class="tabular-nums">{job.user_article_count}</dd>
			</div>
		{/if}
	</dl>

	<section class="pipeline-section">
		<h4 class="kicker">Pipeline</h4>
		<PipelineProgress
			currentStage={job.current_stage}
			stageIndex={job.stage_index}
			stagesCompleted={job.stages_completed}
			subStageProgress={job.sub_stage_progress}
		/>
	</section>

	{#if Object.keys(job.genre_progress).length > 0}
		<GenreProgressList genreProgress={job.genre_progress} />
	{/if}
</article>

<style>
	.active-card {
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
		padding: 1.25rem 1.5rem;
		background: var(--surface-bg);
		border-top: 2px solid var(--alt-charcoal);
		border-bottom: 2px solid var(--alt-charcoal);
		border-left: 1px solid var(--surface-border);
		border-right: 1px solid var(--surface-border);
	}

	.card-head {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.kicker-row {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 1rem;
	}

	.kicker {
		font-family: var(--font-body);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0;
	}

	.job-id {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-slate);
		margin: 0;
		word-break: break-all;
	}

	.meta {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
		gap: 0;
		margin: 0;
		border-top: 1px solid var(--surface-border);
		border-bottom: 1px solid var(--surface-border);
	}

	.meta-cell {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		padding: 0.65rem 0.75rem;
		border-right: 1px solid var(--surface-border);
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
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.pipeline-section {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
</style>

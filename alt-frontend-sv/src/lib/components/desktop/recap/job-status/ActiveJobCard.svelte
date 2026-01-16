<script lang="ts">
	import type { ActiveJobInfo } from "$lib/schema/dashboard";
	import PipelineProgress from "./PipelineProgress.svelte";
	import GenreProgressList from "./GenreProgressList.svelte";
	import StatusBadge from "./StatusBadge.svelte";
	import { Play, Clock, FileText, User } from "@lucide/svelte";

	interface Props {
		job: ActiveJobInfo;
	}

	let { job }: Props = $props();

	const startedAt = $derived(new Date(job.kicked_at).toLocaleString());
	const elapsedTime = $derived(() => {
		const start = new Date(job.kicked_at).getTime();
		const now = Date.now();
		const secs = Math.floor((now - start) / 1000);
		if (secs < 60) return `${secs}s`;
		const mins = Math.floor(secs / 60);
		const remainingSecs = secs % 60;
		return `${mins}m ${remainingSecs}s`;
	});
</script>

<div
	class="p-6 rounded-lg border-2 border-blue-200"
	style="background: var(--surface-bg);"
>
	<div class="flex items-center justify-between mb-4">
		<div class="flex items-center gap-3">
			<div class="w-10 h-10 rounded-lg bg-blue-100 flex items-center justify-center">
				<Play class="w-5 h-5 text-blue-600" />
			</div>
			<div>
				<h3 class="text-lg font-semibold" style="color: var(--text-primary);">
					Active Job
				</h3>
				<p class="text-xs font-mono" style="color: var(--text-muted);">
					{job.job_id}
				</p>
			</div>
		</div>
		<StatusBadge status={job.status} />
	</div>

	<!-- Meta info -->
	<div class="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-6">
		<div class="flex items-center gap-2">
			<Clock class="w-4 h-4" style="color: var(--text-muted);" />
			<div>
				<p class="text-xs" style="color: var(--text-muted);">Started</p>
				<p class="text-sm font-medium" style="color: var(--text-primary);">
					{startedAt}
				</p>
			</div>
		</div>
		<div class="flex items-center gap-2">
			<Clock class="w-4 h-4" style="color: var(--text-muted);" />
			<div>
				<p class="text-xs" style="color: var(--text-muted);">Elapsed</p>
				<p class="text-sm font-medium" style="color: var(--text-primary);">
					{elapsedTime()}
				</p>
			</div>
		</div>
		{#if job.total_articles}
			<div class="flex items-center gap-2">
				<FileText class="w-4 h-4" style="color: var(--text-muted);" />
				<div>
					<p class="text-xs" style="color: var(--text-muted);">Articles</p>
					<p class="text-sm font-medium" style="color: var(--text-primary);">
						{job.total_articles}
					</p>
				</div>
			</div>
		{/if}
		{#if job.user_article_count !== null}
			<div class="flex items-center gap-2">
				<User class="w-4 h-4" style="color: var(--text-muted);" />
				<div>
					<p class="text-xs" style="color: var(--text-muted);">Your Articles</p>
					<p class="text-sm font-medium" style="color: var(--text-primary);">
						{job.user_article_count}
					</p>
				</div>
			</div>
		{/if}
	</div>

	<!-- Pipeline progress -->
	<div class="mb-6">
		<h4 class="text-sm font-semibold mb-3" style="color: var(--text-muted);">
			Pipeline Progress
		</h4>
		<PipelineProgress
			currentStage={job.current_stage}
			stageIndex={job.stage_index}
			stagesCompleted={job.stages_completed}
		/>
	</div>

	<!-- Genre progress -->
	{#if Object.keys(job.genre_progress).length > 0}
		<GenreProgressList genreProgress={job.genre_progress} />
	{/if}
</div>

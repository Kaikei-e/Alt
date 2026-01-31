<script lang="ts">
import type { ActiveJobInfo } from "$lib/schema/dashboard";
import MobilePipelineProgress from "./MobilePipelineProgress.svelte";
import MobileGenreProgressGrid from "./MobileGenreProgressGrid.svelte";
import { StatusBadge } from "$lib/components/desktop/recap/job-status";
import { Play, Clock, ChevronDown, ChevronUp, Activity } from "@lucide/svelte";

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
	const secs = Math.floor((now - start) / 1000);
	if (secs < 60) return `${secs}s`;
	const mins = Math.floor(secs / 60);
	const remainingSecs = secs % 60;
	return `${mins}m ${remainingSecs}s`;
});

// Auto-expand when job is running
$effect(() => {
	if (job) {
		isExpanded = true;
	}
});
</script>

<div class="px-4 mb-4" data-testid="mobile-active-job-panel">
	{#if job}
		<div
			class="rounded-xl border-2 border-blue-200 overflow-hidden"
			style="background: var(--surface-bg);"
		>
			<!-- Header (always visible, tap to toggle) -->
			<button
				class="w-full flex items-center justify-between p-4"
				onclick={() => isExpanded = !isExpanded}
				data-testid="active-job-collapse-toggle"
				aria-expanded={isExpanded}
			>
				<div class="flex items-center gap-3">
					<div class="w-8 h-8 rounded-lg bg-blue-100 flex items-center justify-center">
						<Play class="w-4 h-4 text-blue-600" />
					</div>
					<div class="text-left">
						<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
							Active Job
						</h3>
						<p class="text-xs font-mono" style="color: var(--text-muted);">
							{job.job_id.slice(0, 12)}...
						</p>
					</div>
				</div>
				<div class="flex items-center gap-2">
					<StatusBadge status={job.status} size="sm" />
					{#if isExpanded}
						<ChevronUp class="w-5 h-5" style="color: var(--text-muted);" />
					{:else}
						<ChevronDown class="w-5 h-5" style="color: var(--text-muted);" />
					{/if}
				</div>
			</button>

			<!-- Expandable content -->
			{#if isExpanded}
				<div class="px-4 pb-4 border-t" style="border-color: var(--surface-border);">
					<!-- Meta info -->
					<div class="flex gap-4 py-3 text-sm">
						<div class="flex items-center gap-1">
							<Clock class="w-3 h-3" style="color: var(--text-muted);" />
							<span style="color: var(--text-muted);">Started:</span>
							<span style="color: var(--text-primary);">{startedAt}</span>
						</div>
						<div class="flex items-center gap-1">
							<Clock class="w-3 h-3" style="color: var(--text-muted);" />
							<span style="color: var(--text-muted);">Elapsed:</span>
							<span style="color: var(--text-primary);">{elapsedTime}</span>
						</div>
					</div>

					<!-- Pipeline progress (vertical stepper) -->
					<MobilePipelineProgress
						currentStage={job.current_stage}
						stageIndex={job.stage_index}
						stagesCompleted={job.stages_completed}
						subStageProgress={job.sub_stage_progress}
					/>

					<!-- Genre progress (2-column grid) -->
					{#if Object.keys(job.genre_progress).length > 0}
						<MobileGenreProgressGrid genreProgress={job.genre_progress} />
					{/if}
				</div>
			{/if}
		</div>
	{:else}
		<!-- No job running -->
		<div
			class="p-6 rounded-xl border text-center"
			style="background: var(--surface-bg); border-color: var(--surface-border);"
		>
			<Activity class="w-6 h-6 mx-auto mb-2" style="color: var(--text-muted);" />
			<p class="text-sm" style="color: var(--text-muted);">No job currently running</p>
		</div>
	{/if}
</div>

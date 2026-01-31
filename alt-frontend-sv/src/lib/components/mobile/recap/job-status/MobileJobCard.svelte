<script lang="ts">
import type { RecentJobSummary } from "$lib/schema/dashboard";
import { StatusBadge } from "$lib/components/desktop/recap/job-status";
import { formatDuration, PIPELINE_STAGES } from "$lib/schema/dashboard";
import { Clock, ChevronRight, Server, User } from "@lucide/svelte";

interface Props {
	job: RecentJobSummary;
	onSelect: (job: RecentJobSummary) => void;
}

let { job, onSelect }: Props = $props();

const startedAt = $derived(
	new Date(job.kicked_at).toLocaleString("ja-JP", {
		month: "numeric",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	}),
);

const duration = $derived(formatDuration(job.duration_secs));

// Calculate completed stages count
const completedStages = $derived.by(() => {
	const history = job.status_history || [];
	const completedSet = new Set<string>();
	for (const t of history) {
		if (t.status === "completed" && t.stage) {
			completedSet.add(t.stage);
		}
	}
	return completedSet.size;
});

const totalStages = PIPELINE_STAGES.length;

function handleClick() {
	onSelect(job);
}

function handleKeyDown(e: KeyboardEvent) {
	if (e.key === "Enter" || e.key === " ") {
		e.preventDefault();
		onSelect(job);
	}
}
</script>

<div
	class="p-4 rounded-xl border transition-colors cursor-pointer hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500"
	style="background: var(--surface-bg); border-color: var(--surface-border);"
	data-testid="mobile-job-card"
	role="button"
	tabindex="0"
	onclick={handleClick}
	onkeydown={handleKeyDown}
>
	<div class="flex items-start justify-between mb-2">
		<div class="flex-1 min-w-0">
			<p
				class="text-sm font-mono truncate"
				style="color: var(--text-primary);"
			>
				{job.job_id.slice(0, 16)}...
			</p>
			<p class="text-xs" style="color: var(--text-muted);">
				{startedAt}
			</p>
		</div>
		<div class="flex items-center gap-2 flex-shrink-0">
			<StatusBadge status={job.status} size="sm" />
			<ChevronRight class="w-4 h-4" style="color: var(--text-muted);" />
		</div>
	</div>

	<div class="flex items-center gap-4 text-xs" style="color: var(--text-muted);">
		<!-- Duration -->
		<div class="flex items-center gap-1">
			<Clock class="w-3 h-3" />
			<span>{duration}</span>
		</div>

		<!-- Stages progress -->
		<div class="flex items-center gap-1">
			<span>{completedStages}/{totalStages} stages</span>
		</div>

		<!-- Trigger source -->
		<div class="flex items-center gap-1">
			{#if job.trigger_source === "user"}
				<User class="w-3 h-3" />
			{:else}
				<Server class="w-3 h-3" />
			{/if}
			<span class="capitalize">{job.trigger_source}</span>
		</div>
	</div>
</div>

<script lang="ts">
import type { RecentJobSummary } from "$lib/schema/dashboard";
import {
	StatusBadge,
	StatusTransitionTimeline,
} from "$lib/components/desktop/recap/job-status";
import MobileStageDurationList from "./MobileStageDurationList.svelte";
import { formatDuration } from "$lib/schema/dashboard";
import * as Sheet from "$lib/components/ui/sheet";
import { Clock, Server, User, X } from "@lucide/svelte";

interface Props {
	job: RecentJobSummary | null;
	open: boolean;
	onClose: () => void;
}

let { job, open, onClose }: Props = $props();

const startedAt = $derived(
	job
		? new Date(job.kicked_at).toLocaleString("ja-JP", {
				year: "numeric",
				month: "numeric",
				day: "numeric",
				hour: "2-digit",
				minute: "2-digit",
				second: "2-digit",
			})
		: "",
);

const duration = $derived(job ? formatDuration(job.duration_secs) : "-");
</script>

<Sheet.Root bind:open={open} onOpenChange={(value) => !value && onClose()}>
	<Sheet.Content
		side="bottom"
		class="max-h-[85vh] rounded-t-[24px] border-t border-[var(--border-glass)] shadow-lg w-full max-w-full sm:max-w-full p-0 gap-0 flex flex-col overflow-hidden [&>button.ring-offset-background]:hidden"
		style="background: white !important;"
		data-testid="mobile-job-detail-sheet"
	>
		<!-- Header -->
		<Sheet.Header class="border-b border-[var(--border-glass)] px-4 py-4">
			<div class="flex items-center justify-between">
				<div class="flex-1 min-w-0">
					<Sheet.Title class="text-lg font-bold text-[var(--text-primary)]">
						Job Details
					</Sheet.Title>
					{#if job}
						<Sheet.Description class="text-xs font-mono text-[var(--text-secondary)] truncate">
							{job.job_id}
						</Sheet.Description>
					{/if}
				</div>
				{#if job}
					<StatusBadge status={job.status} />
				{/if}
			</div>
		</Sheet.Header>

		<!-- Scrollable content -->
		<div class="overflow-y-auto flex-1 px-4 py-4 pb-[calc(1rem+env(safe-area-inset-bottom,0px))]">
			{#if job}
				<!-- Meta info -->
				<div class="grid grid-cols-2 gap-3 mb-6">
					<div
						class="p-3 rounded-lg border"
						style="background: var(--surface-bg); border-color: var(--surface-border);"
					>
						<div class="flex items-center gap-2 mb-1">
							<Clock class="w-4 h-4" style="color: var(--text-muted);" />
							<span class="text-xs" style="color: var(--text-muted);">Started</span>
						</div>
						<p class="text-sm font-medium" style="color: var(--text-primary);">
							{startedAt}
						</p>
					</div>
					<div
						class="p-3 rounded-lg border"
						style="background: var(--surface-bg); border-color: var(--surface-border);"
					>
						<div class="flex items-center gap-2 mb-1">
							<Clock class="w-4 h-4" style="color: var(--text-muted);" />
							<span class="text-xs" style="color: var(--text-muted);">Duration</span>
						</div>
						<p class="text-sm font-medium" style="color: var(--text-primary);">
							{duration}
						</p>
					</div>
					<div
						class="p-3 rounded-lg border"
						style="background: var(--surface-bg); border-color: var(--surface-border);"
					>
						<div class="flex items-center gap-2 mb-1">
							{#if job.trigger_source === "user"}
								<User class="w-4 h-4" style="color: var(--text-muted);" />
							{:else}
								<Server class="w-4 h-4" style="color: var(--text-muted);" />
							{/if}
							<span class="text-xs" style="color: var(--text-muted);">Source</span>
						</div>
						<p class="text-sm font-medium capitalize" style="color: var(--text-primary);">
							{job.trigger_source}
						</p>
					</div>
					<div
						class="p-3 rounded-lg border"
						style="background: var(--surface-bg); border-color: var(--surface-border);"
					>
						<div class="flex items-center gap-2 mb-1">
							<span class="text-xs" style="color: var(--text-muted);">Last Stage</span>
						</div>
						<p class="text-sm font-medium" style="color: var(--text-primary);">
							{job.last_stage ?? "-"}
						</p>
					</div>
				</div>

				<!-- Stage Duration Breakdown -->
				<div class="mb-6">
					<MobileStageDurationList
						statusHistory={job.status_history}
						jobStatus={job.status}
						jobKickedAt={job.kicked_at}
					/>
				</div>

				<!-- Status History -->
				<div>
					<h4 class="text-sm font-semibold mb-2" style="color: var(--text-muted);">
						Status History
					</h4>
					<StatusTransitionTimeline transitions={job.status_history} />
				</div>
			{/if}
		</div>

		<!-- Close button -->
		<Sheet.Close
			class="absolute right-4 top-4 h-8 w-8 rounded-full border border-[var(--border-glass)] bg-white text-[var(--text-primary)] hover:bg-gray-100 transition-colors inline-flex shrink-0 items-center justify-center focus-visible:outline-none"
			aria-label="Close"
		>
			<X class="h-4 w-4" />
		</Sheet.Close>
	</Sheet.Content>
</Sheet.Root>

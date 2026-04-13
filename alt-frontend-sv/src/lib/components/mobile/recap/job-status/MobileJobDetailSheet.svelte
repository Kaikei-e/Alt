<script lang="ts">
import type { RecentJobSummary } from "$lib/schema/dashboard";
import { StatusTransitionTimeline } from "$lib/components/desktop/recap/job-status";
import StatusGlyph from "$lib/components/recap/job-status/StatusGlyph.svelte";
import MobileStageDurationList from "./MobileStageDurationList.svelte";
import { formatDuration } from "$lib/schema/dashboard";
import * as Sheet from "$lib/components/ui/sheet";
import { X } from "@lucide/svelte";

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

const duration = $derived(job ? formatDuration(job.duration_secs) : "—");
</script>

<Sheet.Root bind:open onOpenChange={(value) => !value && onClose()}>
	<Sheet.Content
		side="bottom"
		class="max-h-[85vh] w-full max-w-full sm:max-w-full p-0 gap-0 flex flex-col overflow-hidden alt-paper-sheet [&>button.ring-offset-background]:hidden"
		data-testid="mobile-job-detail-sheet"
	>
		<Sheet.Header class="sheet-head">
			<div class="head-row">
				<div class="head-text">
					<Sheet.Title class="sheet-title">Job details</Sheet.Title>
					{#if job}
						<Sheet.Description class="sheet-id">
							{job.job_id}
						</Sheet.Description>
					{/if}
				</div>
				{#if job}
					<StatusGlyph
						status={job.status}
						pulse={job.status === "running"}
						includeLabel={true}
					/>
				{/if}
			</div>
		</Sheet.Header>

		<div class="sheet-body">
			{#if job}
				<dl class="meta">
					<div class="meta-cell">
						<dt>Started</dt>
						<dd class="tabular-nums">{startedAt}</dd>
					</div>
					<div class="meta-cell">
						<dt>Duration</dt>
						<dd class="tabular-nums">{duration}</dd>
					</div>
					<div class="meta-cell">
						<dt>Source</dt>
						<dd>{job.trigger_source === "user" ? "User" : "System"}</dd>
					</div>
					<div class="meta-cell">
						<dt>Last stage</dt>
						<dd>{job.last_stage ?? "—"}</dd>
					</div>
				</dl>

				<section class="detail-section">
					<MobileStageDurationList
						statusHistory={job.status_history}
						jobStatus={job.status}
						jobKickedAt={job.kicked_at}
					/>
				</section>

				<section class="detail-section">
					<h4 class="kicker">Status history</h4>
					<StatusTransitionTimeline transitions={job.status_history} />
				</section>
			{/if}
		</div>

		<Sheet.Close class="sheet-close" aria-label="Close">
			<X class="h-4 w-4" />
		</Sheet.Close>
	</Sheet.Content>
</Sheet.Root>

<style>
	:global(.alt-paper-sheet) {
		background: var(--surface-bg) !important;
		border-top: 2px solid var(--alt-charcoal) !important;
		border-radius: 0 !important;
	}

	:global(.sheet-head) {
		padding: 1rem 1rem 0.85rem;
		border-bottom: 1px solid var(--surface-border);
	}

	.head-row {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.head-text {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		min-width: 0;
		flex: 1;
	}

	:global(.sheet-title) {
		font-family: var(--font-display);
		font-size: 1.1rem;
		font-weight: 700;
		color: var(--alt-charcoal);
	}

	:global(.sheet-id) {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-slate);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.sheet-body {
		flex: 1;
		overflow-y: auto;
		padding: 1rem 1rem
			calc(1.5rem + env(safe-area-inset-bottom, 0px));
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
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
		padding: 0.55rem 0.6rem;
		border-right: 1px solid var(--surface-border);
		border-bottom: 1px solid var(--surface-border);
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}

	.meta-cell:nth-child(2n) {
		border-right: none;
	}

	.meta-cell:nth-last-child(-n + 2) {
		border-bottom: none;
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

	.detail-section {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
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

	:global(.sheet-close) {
		position: absolute;
		right: 0.85rem;
		top: 0.85rem;
		width: 32px;
		height: 32px;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
		color: var(--alt-charcoal);
		display: inline-flex;
		align-items: center;
		justify-content: center;
		cursor: pointer;
	}

	:global(.sheet-close:hover) {
		background: var(--surface-hover);
	}
</style>

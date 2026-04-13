<script lang="ts">
import type { StatusTransition } from "$lib/schema/dashboard";
import StatusGlyph from "$lib/components/recap/job-status/StatusGlyph.svelte";

interface Props {
	transitions: StatusTransition[];
}

let { transitions }: Props = $props();

function formatTime(isoString: string): string {
	return new Date(isoString).toLocaleTimeString("ja-JP", {
		hour: "2-digit",
		minute: "2-digit",
		second: "2-digit",
	});
}
</script>

<div class="timeline" data-role="status-timeline">
	{#if transitions.length === 0}
		<p class="empty">No status history.</p>
	{:else}
		<ol class="entries">
			{#each transitions as transition}
				<li class="entry" data-status={transition.status}>
					<span class="time tabular-nums">{formatTime(transition.transitioned_at)}</span>
					<span class="rule" aria-hidden="true"></span>
					<span class="status-cell">
						<StatusGlyph
							status={transition.status}
							pulse={transition.status === "running"}
							includeLabel={true}
						/>
					</span>
					{#if transition.stage}
						<span class="stage">@ {transition.stage}</span>
					{/if}
					{#if transition.reason}
						<span class="reason" title={transition.reason}>
							{transition.reason}
						</span>
					{/if}
				</li>
			{/each}
		</ol>
	{/if}
</div>

<style>
	.timeline {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.entries {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.entry {
		display: grid;
		grid-template-columns: 5rem 1px auto auto 1fr;
		align-items: baseline;
		gap: 0.6rem;
		padding: 0.3rem 0;
		border-bottom: 1px solid var(--surface-border);
	}

	.entry:last-child {
		border-bottom: none;
	}

	.time {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-slate);
	}

	.rule {
		display: block;
		height: 100%;
		min-height: 1rem;
		background: var(--surface-border);
	}

	.stage {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-slate);
	}

	.reason {
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-style: italic;
		color: var(--alt-slate);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.empty {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-slate);
		margin: 0;
	}
</style>

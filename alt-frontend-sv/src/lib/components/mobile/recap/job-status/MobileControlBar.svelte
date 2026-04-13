<script lang="ts">
interface Props {
	onRefresh: () => void;
	onTriggerJob: () => void;
	loading: boolean;
	triggering: boolean;
	hasRunningJob: boolean;
	justStartedJobId: string | null;
}

let {
	onRefresh,
	onTriggerJob,
	loading,
	triggering,
	hasRunningJob,
	justStartedJobId,
}: Props = $props();

const isStartDisabled = $derived(
	triggering || hasRunningJob || justStartedJobId !== null,
);

const startButtonTooltip = $derived.by(() => {
	if (justStartedJobId) return "Job is starting…";
	if (hasRunningJob) return "A job is already running";
	return "Start a new recap job";
});
</script>

<div
	class="control-bar"
	data-testid="mobile-control-bar"
	data-role="control-bar"
>
	<button
		type="button"
		class="bar-button"
		onclick={onRefresh}
		disabled={loading}
		aria-label="Refresh job data"
		data-role="refresh"
	>
		{loading ? "Refreshing…" : "Refresh"}
	</button>

	<button
		type="button"
		class="bar-button bar-button--primary"
		onclick={onTriggerJob}
		disabled={isStartDisabled}
		title={startButtonTooltip}
		aria-label="Start new recap job"
		data-role="start-job"
	>
		{triggering ? "Starting…" : "Start job"}
	</button>
</div>

<style>
	.control-bar {
		position: fixed;
		left: 0;
		right: 0;
		bottom: calc(2.75rem + env(safe-area-inset-bottom, 0px));
		z-index: 50;
		display: flex;
		gap: 0.75rem;
		padding: 0.75rem 1rem
			calc(0.75rem + env(safe-area-inset-bottom, 0px));
		background: var(--surface-bg);
		border-top: 1px solid var(--surface-border);
	}

	.bar-button {
		all: unset;
		flex: 1;
		min-height: 48px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		border: 1.5px solid var(--alt-charcoal);
		color: var(--alt-charcoal);
		background: transparent;
		cursor: pointer;
		transition:
			background 0.15s ease,
			color 0.15s ease;
	}

	.bar-button:hover:not(:disabled) {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.bar-button:focus-visible {
		outline: 2px solid var(--alt-charcoal);
		outline-offset: 2px;
	}

	.bar-button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.bar-button--primary {
		border-width: 2px;
	}

	.bar-button--primary:not(:disabled) {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.bar-button--primary:hover:not(:disabled) {
		background: var(--surface-bg);
		color: var(--alt-charcoal);
	}
</style>

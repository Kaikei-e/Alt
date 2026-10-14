<script lang="ts">
import { AlertCircle, RefreshCw, WifiOff } from "@lucide/svelte";

interface Props {
	pendingCount: number;
	isConnected: boolean;
	isFallback: boolean;
	onApply: () => void;
}

const { pendingCount, isConnected, isFallback, onApply }: Props = $props();

let hasConnectedOnce = $state(false);
$effect(() => {
	if (isConnected) {
		hasConnectedOnce = true;
	}
});

const indicatorColor = $derived(
	isFallback
		? "var(--alt-error)"
		: isConnected
			? "var(--alt-success)"
			: "var(--alt-ash)",
);
const indicatorPulse = $derived(isConnected && !isFallback);
</script>

{#if pendingCount > 0}
	<div class="update-bar update-bar--pending">
		<span
			class="indicator {indicatorPulse ? 'indicator--pulse' : ''}"
			style="background: {indicatorColor};"
			aria-hidden="true"
		></span>
		<button class="update-apply" onclick={onApply}>
			<RefreshCw class="h-3.5 w-3.5" />
			{pendingCount} {pendingCount === 1 ? 'item' : 'items'} updated
		</button>
	</div>
{:else if isFallback}
	<div class="update-bar update-bar--fallback">
		<AlertCircle class="h-3.5 w-3.5" style="color: var(--alt-error);" />
		<span class="update-text">Live updates unavailable</span>
	</div>
{:else if hasConnectedOnce && !isConnected}
	<div class="update-bar update-bar--disconnected">
		<WifiOff class="h-3.5 w-3.5" style="color: var(--alt-ash);" />
		<span class="update-text update-text--muted">Reconnecting...</span>
	</div>
{/if}

<style>
	.update-bar {
		width: 100%;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-family: var(--font-body);
		font-size: 0.8rem;
	}

	.update-bar--pending {
		padding: 0.5rem 0.9rem;
		border: 1px solid color-mix(in srgb, var(--alt-primary) 35%, transparent);
		background: var(--action-surface);
	}

	.update-bar--fallback,
	.update-bar--disconnected {
		padding: 0.15rem 0;
	}

	.indicator {
		width: 0.625rem;
		height: 0.625rem;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.indicator--pulse {
		animation: pulse 1.2s ease-in-out infinite;
	}

	.update-apply {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--interactive-text);
		background: transparent;
		border: none;
		cursor: pointer;
		transition: opacity 0.15s;
	}

	.update-apply:hover {
		opacity: 0.8;
	}

	.update-text {
		font-style: italic;
		color: var(--alt-slate);
	}

	.update-text--muted {
		color: var(--alt-ash);
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}
</style>

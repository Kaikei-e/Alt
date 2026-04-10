<script lang="ts">
import { AlertCircle, RefreshCw, WifiOff } from "@lucide/svelte";

interface Props {
	pendingCount: number;
	isConnected: boolean;
	isFallback: boolean;
	onApply: () => void;
}

const { pendingCount, isConnected, isFallback, onApply }: Props = $props();

const indicatorColor = $derived(
	isFallback
		? "var(--badge-orange-text)"
		: isConnected
			? "var(--badge-green-text)"
			: "var(--badge-gray-text)",
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
		<AlertCircle class="h-3.5 w-3.5" style="color: var(--badge-orange-text);" />
		<span class="update-text">Live updates unavailable</span>
	</div>
{:else if !isConnected}
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
		padding: 0.5rem 1rem;
		font-size: 0.875rem;
		border: 1px solid var(--surface-border);
	}

	.update-bar--pending {
		border-color: color-mix(in srgb, var(--alt-primary) 30%, transparent);
		background: color-mix(in srgb, var(--alt-primary) 5%, transparent);
	}

	.update-bar--fallback {
		border-color: var(--badge-orange-border);
		background: var(--badge-orange-bg);
	}

	.update-bar--disconnected {
		background: var(--surface-bg);
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
		color: var(--alt-primary);
		background: transparent;
		border: none;
		cursor: pointer;
		transition: opacity 0.15s;
	}

	.update-apply:hover {
		opacity: 0.8;
	}

	.update-text {
		font-family: var(--font-body);
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

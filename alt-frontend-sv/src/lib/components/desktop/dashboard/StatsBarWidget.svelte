<script lang="ts">
interface Props {
	feedAmount: number;
	totalArticlesAmount: number;
	unsummarizedArticlesAmount: number;
	isConnected: boolean;
}

const {
	feedAmount,
	totalArticlesAmount,
	unsummarizedArticlesAmount,
	isConnected,
}: Props = $props();

let summarizedArticles = $derived(
	totalArticlesAmount - unsummarizedArticlesAmount,
);
</script>

<div class="figures-bar">
	<div class="figure-group">
		<span class="figure-label">FEEDS</span>
		<span class="figure-value">{feedAmount.toLocaleString()}</span>
	</div>

	<div class="figure-separator" aria-hidden="true"></div>

	<div class="figure-group">
		<span class="figure-label">ARTICLES</span>
		<span class="figure-value">{totalArticlesAmount.toLocaleString()}</span>
	</div>

	<div class="figure-separator" aria-hidden="true"></div>

	<div class="figure-group">
		<span class="figure-label">SUMMARIZED</span>
		<span class="figure-value">{summarizedArticles.toLocaleString()}</span>
	</div>

	<div class="status-group">
		<span
			class="status-dot"
			class:status-dot--live={isConnected}
			class:status-dot--offline={!isConnected}
		></span>
		<span class="status-label">{isConnected ? "Live" : "Offline"}</span>
	</div>
</div>

<style>
	.figures-bar {
		display: flex;
		align-items: baseline;
		gap: 1.5rem;
		padding: 0.75rem 0;
	}

	.figure-group {
		display: flex;
		align-items: baseline;
		gap: 0.4rem;
	}

	.figure-label {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.figure-value {
		font-family: var(--font-mono);
		font-size: 1.1rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		font-variant-numeric: tabular-nums;
	}

	.figure-separator {
		width: 1px;
		height: 1.2rem;
		background: var(--surface-border);
		flex-shrink: 0;
	}

	.status-group {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		margin-left: auto;
	}

	.status-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.status-dot--live {
		background: var(--alt-sage);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.status-dot--offline {
		background: var(--alt-ash);
		animation: none;
	}

	.status-label {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
		}
		50% {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.status-dot--live {
			animation: none;
			opacity: 1;
		}
	}
</style>

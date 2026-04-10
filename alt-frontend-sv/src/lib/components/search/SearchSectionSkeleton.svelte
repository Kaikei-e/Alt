<script lang="ts">
interface Props {
	label?: string;
	rows?: number;
}

const { label = "Loading", rows = 3 }: Props = $props();
</script>

<section data-role="skeleton-section">
	<div class="skeleton-label" aria-label={label}></div>
	{#each Array(rows) as _}
		<div class="skeleton-card">
			<div class="skeleton-bar skeleton-bar--wide"></div>
			<div class="skeleton-bar skeleton-bar--full"></div>
			<div class="skeleton-bar skeleton-bar--half"></div>
		</div>
	{/each}
</section>

<style>
	.skeleton-label {
		width: 6rem;
		height: 0.65rem;
		background: var(--surface-hover);
		margin-bottom: 0.5rem;
	}

	.skeleton-card {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.75rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
		margin-bottom: 0.5rem;
	}

	.skeleton-bar {
		height: 0.75rem;
		background: var(--surface-hover);
		animation: skeleton-pulse 1.2s ease-in-out infinite;
	}

	.skeleton-bar--wide {
		width: 75%;
	}

	.skeleton-bar--full {
		width: 100%;
	}

	.skeleton-bar--half {
		width: 50%;
	}

	@keyframes skeleton-pulse {
		0%,
		100% {
			opacity: 0.4;
		}
		50% {
			opacity: 0.8;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.skeleton-bar {
			animation: none;
			opacity: 0.6;
		}
	}
</style>

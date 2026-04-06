<script lang="ts">
interface Props {
	label?: string;
	rows?: number;
}

const { label = "Loading", rows = 3 }: Props = $props();
</script>

<section class="space-y-3">
	<div
		class="h-4 w-24 rounded skeleton-pulse"
		aria-label="{label}"
	></div>
	{#each Array(rows) as _}
		<div
			class="rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] p-4 space-y-2"
		>
			<div class="h-4 w-3/4 rounded skeleton-pulse"></div>
			<div class="h-3 w-full rounded skeleton-pulse"></div>
			<div class="h-3 w-1/2 rounded skeleton-pulse"></div>
		</div>
	{/each}
</section>

<style>
	.skeleton-pulse {
		background: linear-gradient(
			90deg,
			var(--surface-hover) 25%,
			var(--surface-bg) 50%,
			var(--surface-hover) 75%
		);
		background-size: 200% 100%;
		animation: pulse 1.5s ease-in-out infinite;
	}
	@keyframes pulse {
		0% {
			background-position: 200% 0;
		}
		100% {
			background-position: -200% 0;
		}
	}
</style>

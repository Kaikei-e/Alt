<script lang="ts">
type Props = {
	minHeight?: string;
};

let { minHeight = "60dvh" }: Props = $props();
</script>

<div class="skeleton-container" style="min-height: {minHeight};">
	<!-- Lede placeholder -->
	<div class="skeleton-group">
		<div class="skeleton-bar" style="width: 100%; height: 1.1rem;"></div>
		<div class="skeleton-bar" style="width: 85%; height: 1.1rem;"></div>
	</div>

	<!-- Section 1 -->
	<div class="skeleton-section">
		<div class="skeleton-rule"></div>
		<div class="skeleton-bar" style="width: 10rem; height: 1.15rem;"></div>
		<div class="skeleton-group">
			<div class="skeleton-bullet">
				<div class="skeleton-bar" style="width: 100%; height: 0.9rem;"></div>
			</div>
			<div class="skeleton-bullet">
				<div class="skeleton-bar" style="width: 92%; height: 0.9rem;"></div>
			</div>
			<div class="skeleton-bullet">
				<div class="skeleton-bar" style="width: 75%; height: 0.9rem;"></div>
			</div>
		</div>
	</div>

	<!-- Section 2 -->
	<div class="skeleton-section">
		<div class="skeleton-rule"></div>
		<div class="skeleton-bar" style="width: 8rem; height: 1.15rem;"></div>
		<div class="skeleton-group">
			<div class="skeleton-bullet">
				<div class="skeleton-bar" style="width: 100%; height: 0.9rem;"></div>
			</div>
			<div class="skeleton-bullet">
				<div class="skeleton-bar" style="width: 88%; height: 0.9rem;"></div>
			</div>
		</div>
	</div>
</div>

<style>
	.skeleton-container {
		padding: 0;
	}

	.skeleton-bar {
		background: var(--skeleton-base, #e8e4de);
		animation: shimmer 1.8s ease-in-out infinite;
	}

	.skeleton-group {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin-bottom: 2rem;
	}

	.skeleton-section {
		margin-bottom: 1.5rem;
	}

	.skeleton-rule {
		height: 1px;
		background: var(--skeleton-base, #e8e4de);
		margin-bottom: 1rem;
	}

	.skeleton-section .skeleton-bar {
		margin-bottom: 0.75rem;
	}

	.skeleton-bullet {
		padding-left: 0.75rem;
		border-left: 1px solid var(--skeleton-base, #e8e4de);
	}

	@keyframes shimmer {
		0%, 100% { opacity: 0.5; }
		50% { opacity: 1; }
	}

	@media (prefers-reduced-motion: reduce) {
		.skeleton-bar { animation: none; opacity: 0.6; }
	}
</style>

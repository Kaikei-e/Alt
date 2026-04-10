<script lang="ts">
const skeletonCards = [1, 2, 3];
</script>

<div class="flex flex-col gap-3">
	<!-- TodayBar skeleton (2-row) -->
	<div class="skel-bar">
		<!-- Row 1: Action shortcuts -->
		<div class="skel-bar-actions">
			{#each [1, 2, 3] as _}
				<div class="skel-block skel-block--action"></div>
			{/each}
		</div>
		<!-- Row 2: Stats + tags -->
		<div class="skel-bar-figures">
			{#each [1, 2, 3] as _}
				<div class="skel-block skel-block--stat"></div>
			{/each}
			<div class="flex-1"></div>
			{#each [1, 2] as _}
				<div class="skel-block skel-block--tag"></div>
			{/each}
		</div>
	</div>

	<!-- UnifiedIntentBox skeleton -->
	<div class="px-4 py-2 bg-[var(--surface-bg)]">
		<div class="skel-block skel-block--input"></div>
	</div>

	<!-- Card skeletons -->
	{#each skeletonCards as _, i}
		<div class="skel-card" style="animation-delay: {i * 0.1}s">
			<!-- Title + time -->
			<div class="flex items-start justify-between mb-2">
				<div class="skel-block skel-block--title"></div>
				<div class="skel-block skel-block--time"></div>
			</div>
			<!-- Badges -->
			<div class="flex gap-1 mb-2">
				<div class="skel-block skel-block--badge"></div>
				<div class="skel-block skel-block--badge skel-block--badge-wide"></div>
			</div>
			<!-- Summary skeleton lines -->
			<div class="space-y-1 mb-2">
				<div class="skel-block skel-block--line"></div>
				<div class="skel-block skel-block--line skel-block--line-short"></div>
			</div>
			<!-- Bottom row: tags + actions -->
			<div class="flex items-center justify-between">
				<div class="flex gap-1">
					{#each [1, 2, 3] as _}
						<div class="skel-block skel-block--tag"></div>
					{/each}
				</div>
				<div class="flex gap-1">
					{#each [1, 2, 3] as _}
						<div class="skel-block skel-block--action-sm"></div>
					{/each}
				</div>
			</div>
		</div>
	{/each}
</div>

<style>
	.skel-bar {
		display: flex;
		flex-direction: column;
		border-bottom: 1px solid var(--surface-border);
		background: var(--surface-bg);
	}

	.skel-bar-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 1rem;
		border-bottom: 1px solid color-mix(in srgb, var(--surface-border) 30%, transparent);
	}

	.skel-bar-figures {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.5rem 1rem;
	}

	.skel-card {
		border: 1px solid var(--surface-border);
		border-left: 3px solid var(--surface-border);
		padding: 1rem;
		background: var(--surface-bg);
	}

	.skel-block {
		background: var(--surface-hover);
		animation: skel-pulse 1.2s ease-in-out infinite;
	}

	.skel-block--action { height: 1.75rem; width: 6rem; }
	.skel-block--stat { height: 1rem; width: 4rem; }
	.skel-block--tag { height: 1.25rem; width: 3rem; }
	.skel-block--input { height: 2.25rem; width: 100%; }
	.skel-block--title { height: 1rem; width: 75%; }
	.skel-block--time { height: 0.75rem; width: 3rem; }
	.skel-block--badge { height: 1.25rem; width: 3.5rem; }
	.skel-block--badge-wide { width: 5rem; }
	.skel-block--line { height: 0.75rem; width: 100%; }
	.skel-block--line-short { width: 66%; }
	.skel-block--action-sm { height: 1.5rem; width: 3rem; }

	@keyframes skel-pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}
</style>

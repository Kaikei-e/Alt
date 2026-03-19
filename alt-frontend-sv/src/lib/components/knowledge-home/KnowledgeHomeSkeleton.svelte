<script lang="ts">
const skeletonCards = [1, 2, 3];
</script>

<div class="flex flex-col gap-3">
	<!-- TodayBar skeleton (2-row) -->
	<div class="flex flex-col border-b border-[var(--surface-border)] bg-[var(--surface-bg)]">
		<!-- Row 1: Action shortcuts -->
		<div class="flex items-center gap-2 px-4 py-2 border-b border-[var(--surface-border)]/30">
			{#each [1, 2, 3] as _}
				<div class="h-7 w-24 rounded-md animate-shimmer"></div>
			{/each}
		</div>
		<!-- Row 2: Stats + tags -->
		<div class="flex items-center gap-3 px-4 py-2">
			{#each [1, 2, 3] as _}
				<div class="h-4 w-16 rounded animate-shimmer"></div>
			{/each}
			<div class="flex-1"></div>
			{#each [1, 2] as _}
				<div class="h-5 w-12 rounded animate-shimmer"></div>
			{/each}
		</div>
	</div>

	<!-- UnifiedIntentBox skeleton -->
	<div class="px-4 py-2 bg-[var(--surface-bg)]">
		<div class="h-9 w-full rounded-lg animate-shimmer"></div>
	</div>

	<!-- Card skeletons -->
	{#each skeletonCards as _, i}
		<div
			class="border rounded-lg p-4 bg-[var(--surface-bg)] border-[var(--surface-border)]"
			style="animation-delay: {i * 0.1}s"
		>
			<!-- Title + time -->
			<div class="flex items-start justify-between mb-2">
				<div class="h-4 w-3/4 rounded animate-shimmer"></div>
				<div class="h-3 w-12 rounded animate-shimmer"></div>
			</div>
			<!-- Badges -->
			<div class="flex gap-1 mb-2">
				<div class="h-5 w-14 rounded-full animate-shimmer"></div>
				<div class="h-5 w-20 rounded-full animate-shimmer"></div>
			</div>
			<!-- Summary skeleton lines -->
			<div class="space-y-1 mb-2">
				<div class="h-3 w-full rounded animate-shimmer"></div>
				<div class="h-3 w-2/3 rounded animate-shimmer"></div>
			</div>
			<!-- Bottom row: tags + actions -->
			<div class="flex items-center justify-between">
				<div class="flex gap-1">
					{#each [1, 2, 3] as _}
						<div class="h-5 w-10 rounded animate-shimmer"></div>
					{/each}
				</div>
				<div class="flex gap-1">
					{#each [1, 2, 3] as _}
						<div class="h-6 w-12 rounded animate-shimmer"></div>
					{/each}
				</div>
			</div>
		</div>
	{/each}
</div>

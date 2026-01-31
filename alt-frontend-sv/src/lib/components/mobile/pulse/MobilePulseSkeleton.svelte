<script lang="ts">
// Skeleton loading state for Evening Pulse
</script>

<div class="p-4 max-w-2xl mx-auto" data-testid="pulse-skeleton">
	<!-- Header skeleton -->
	<div class="mb-6">
		<div
			class="h-8 w-48 rounded animate-pulse mb-2"
			style="background: var(--surface-hover);"
		></div>
		<div
			class="h-4 w-32 rounded animate-pulse"
			style="background: var(--surface-hover);"
		></div>
	</div>

	<!-- Card skeletons -->
	<div class="flex flex-col gap-4">
		{#each Array(3) as _, i}
			<div
				class="p-4 rounded-2xl border-2 animate-pulse"
				style="background: var(--surface-bg); border-color: var(--border);"
			>
				<!-- Role label skeleton -->
				<div class="flex justify-between items-start mb-3">
					<div
						class="h-5 w-20 rounded-full"
						style="background: var(--surface-hover);"
					></div>
					<div
						class="h-4 w-16 rounded"
						style="background: var(--surface-hover);"
					></div>
				</div>

				<!-- Title skeleton -->
				<div
					class="h-6 w-3/4 rounded mb-2"
					style="background: var(--surface-hover);"
				></div>

				<!-- Rationale skeleton -->
				<div class="space-y-1 mb-3">
					<div
						class="h-4 w-full rounded"
						style="background: var(--surface-hover);"
					></div>
					<div
						class="h-4 w-2/3 rounded"
						style="background: var(--surface-hover);"
					></div>
				</div>

				<!-- Footer skeleton -->
				<div class="flex gap-3">
					<div
						class="h-3 w-16 rounded"
						style="background: var(--surface-hover);"
					></div>
					<div
						class="h-3 w-14 rounded"
						style="background: var(--surface-hover);"
					></div>
				</div>
			</div>
		{/each}
	</div>
</div>

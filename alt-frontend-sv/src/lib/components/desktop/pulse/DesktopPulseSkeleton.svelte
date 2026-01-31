<script lang="ts">
// Desktop skeleton loading state for Evening Pulse
</script>

<div class="p-6 max-w-7xl mx-auto" data-testid="desktop-pulse-skeleton">
	<!-- Header skeleton -->
	<div class="mb-8">
		<div
			class="h-9 w-48 animate-pulse mb-2"
			style="background: var(--surface-hover);"
		></div>
		<div
			class="h-4 w-64 animate-pulse"
			style="background: var(--surface-hover);"
		></div>
	</div>

	<!-- Card skeletons - 3 column grid -->
	<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
		{#each Array(3) as _, i}
			<div
				class="p-5 border-2 animate-pulse"
				style="background: white; border-color: var(--surface-border); border-left-width: 4px;"
			>
				<!-- Header skeleton -->
				<div class="flex justify-between items-start mb-3">
					<div class="flex items-center gap-2">
						<div
							class="h-5 w-5 rounded-full"
							style="background: var(--surface-hover);"
						></div>
						<div
							class="h-5 w-20 rounded-full"
							style="background: var(--surface-hover);"
						></div>
					</div>
					<div
						class="h-4 w-16"
						style="background: var(--surface-hover);"
					></div>
				</div>

				<!-- Title skeleton -->
				<div
					class="h-6 w-full mb-2"
					style="background: var(--surface-hover);"
				></div>
				<div
					class="h-6 w-3/4 mb-4"
					style="background: var(--surface-hover);"
				></div>

				<!-- Rationale skeleton -->
				<div class="space-y-1.5 mb-4">
					<div
						class="h-4 w-full"
						style="background: var(--surface-hover);"
					></div>
					<div
						class="h-4 w-5/6"
						style="background: var(--surface-hover);"
					></div>
					<div class="flex items-center gap-2 mt-2">
						<div
							class="h-2 w-8"
							style="background: var(--surface-hover);"
						></div>
						<div
							class="h-3 w-20"
							style="background: var(--surface-hover);"
						></div>
					</div>
				</div>

				<!-- Footer skeleton -->
				<div class="flex gap-3">
					<div
						class="h-3 w-16"
						style="background: var(--surface-hover);"
					></div>
					<div
						class="h-3 w-14"
						style="background: var(--surface-hover);"
					></div>
				</div>
			</div>
		{/each}
	</div>
</div>

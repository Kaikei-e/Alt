<script lang="ts">
</script>

<div class="p-6 max-w-7xl mx-auto" data-testid="desktop-pulse-skeleton">
	<h1 class="sr-only">Evening Pulse</h1>
	<div
		class="pb-4 mb-6 border-b"
		style="border-color: var(--surface-border);"
	>
		<div
			class="h-3 w-36 animate-pulse mb-2"
			style="background: var(--skeleton-base);"
		></div>
		<div
			class="h-8 w-48 animate-pulse mb-2"
			style="background: var(--skeleton-base);"
		></div>
		<div
			class="h-4 w-64 animate-pulse"
			style="background: var(--skeleton-base);"
		></div>
	</div>

	<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
		{#each Array(3) as _}
			<div
				class="p-5 border animate-pulse"
				style="background: var(--surface-bg); border-color: var(--surface-border); border-left-width: 3px;"
			>
				<div class="flex justify-between items-start mb-3">
					<div class="flex items-center gap-2">
						<div
							class="h-5 w-6"
							style="background: var(--skeleton-base);"
						></div>
						<div
							class="h-5 w-20"
							style="background: var(--skeleton-base);"
						></div>
					</div>
					<div
						class="h-4 w-16"
						style="background: var(--skeleton-base);"
					></div>
				</div>

				<div
					class="h-6 w-full mb-2"
					style="background: var(--skeleton-base);"
				></div>
				<div
					class="h-6 w-3/4 mb-4"
					style="background: var(--skeleton-base);"
				></div>

				<div class="space-y-1.5 mb-4">
					<div
						class="h-4 w-full"
						style="background: var(--skeleton-base);"
					></div>
					<div
						class="h-4 w-5/6"
						style="background: var(--skeleton-base);"
					></div>
					<div class="flex items-center gap-2 mt-2">
						<div
							class="h-2 w-8"
							style="background: var(--skeleton-base);"
						></div>
						<div
							class="h-3 w-20"
							style="background: var(--skeleton-base);"
						></div>
					</div>
				</div>

				<div class="flex gap-3">
					<div
						class="h-3 w-16"
						style="background: var(--skeleton-base);"
					></div>
					<div
						class="h-3 w-14"
						style="background: var(--skeleton-base);"
					></div>
				</div>
			</div>
		{/each}
	</div>
</div>

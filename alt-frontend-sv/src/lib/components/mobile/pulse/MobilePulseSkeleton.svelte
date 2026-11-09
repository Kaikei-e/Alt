<script lang="ts">
</script>

<div class="p-4 max-w-2xl mx-auto" data-testid="pulse-skeleton">
	<div
		class="pb-4 mb-6 border-b"
		style="border-color: var(--surface-border);"
	>
		<div
			class="h-3 w-28 animate-pulse mb-2"
			style="background: var(--skeleton-base);"
		></div>
		<div
			class="h-7 w-48 animate-pulse mb-2"
			style="background: var(--skeleton-base);"
		></div>
		<div
			class="h-4 w-56 animate-pulse"
			style="background: var(--skeleton-base);"
		></div>
	</div>

	<div class="flex flex-col gap-4">
		{#each Array(3) as _}
			<div
				class="p-4 border animate-pulse"
				style="background: var(--surface-bg); border-color: var(--surface-border); border-left-width: 3px;"
			>
				<div class="flex justify-between items-start mb-3">
					<div
						class="h-5 w-20"
						style="background: var(--skeleton-base);"
					></div>
					<div
						class="h-4 w-16"
						style="background: var(--skeleton-base);"
					></div>
				</div>

				<div
					class="h-6 w-3/4 mb-2"
					style="background: var(--skeleton-base);"
				></div>

				<div class="space-y-1 mb-3">
					<div
						class="h-4 w-full"
						style="background: var(--skeleton-base);"
					></div>
					<div
						class="h-4 w-2/3"
						style="background: var(--skeleton-base);"
					></div>
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

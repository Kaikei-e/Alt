<script lang="ts">
import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
import ViewedFeedsClient from "$lib/components/mobile/ViewedFeedsClient.svelte";
</script>

<div
	class="h-screen overflow-hidden flex flex-col"
	style="background: var(--app-bg);"
>
	<!-- Page Title -->
	<div class="px-5 pt-4 pb-2">
		<h1
			class="text-2xl font-bold text-center"
			style="color: var(--alt-primary); font-family: var(--font-outfit, sans-serif);"
			data-testid="read-feeds-title"
		>
			History
		</h1>
	</div>

	<div class="flex-1 min-h-0 flex flex-col">
		<ViewedFeedsClient />
	</div>
	<FloatingMenu />
</div>


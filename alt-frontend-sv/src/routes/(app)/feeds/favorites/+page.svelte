<script lang="ts">
import { useViewport } from "$lib/stores/viewport.svelte";

// Desktop components
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";

import type { RenderFeed } from "$lib/schema/feed";

const { isDesktop } = useViewport();

let selectedFeed = $state<RenderFeed | null>(null);
let isModalOpen = $state(false);

// TODO: Implement favorites API
const favorites = $state<RenderFeed[]>([]);

function handleSelectFeed(feed: RenderFeed) {
	selectedFeed = feed;
	isModalOpen = true;
}
</script>

<svelte:head>
	<title>Favorites - Alt</title>
</svelte:head>

{#if isDesktop}
	<PageHeader title="Favorites" description="Your starred feeds" />

	<div class="text-center py-12">
		<p class="text-[var(--text-secondary)] text-sm">
			Favorites feature coming soon
		</p>
	</div>

	<FeedDetailModal
		bind:open={isModalOpen}
		feed={selectedFeed}
		onOpenChange={(open) => (isModalOpen = open)}
	/>
{:else}
	<div class="min-h-screen flex flex-col" style="background: var(--app-bg);">
		<div class="px-4 pt-6 pb-4">
			<h1 class="text-xl font-bold text-[var(--text-primary)]">Favorites</h1>
		</div>
		<div class="text-center py-12">
			<p class="text-[var(--text-secondary)] text-sm">
				Favorites feature coming soon
			</p>
		</div>
	</div>
{/if}

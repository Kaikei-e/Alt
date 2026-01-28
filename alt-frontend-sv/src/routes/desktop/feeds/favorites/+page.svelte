<script lang="ts">
	import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
	import type { RenderFeed } from "$lib/schema/feed";
	import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";

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

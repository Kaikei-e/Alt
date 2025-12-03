<script lang="ts">
import type { RenderFeed } from "$lib/schema/feed";
import FeedCard from "./FeedCard.svelte";

interface Props {
	feeds: RenderFeed[];
	readFeeds: Set<string>;
	onMarkAsRead: (feedLink: string) => void;
}

const { feeds, readFeeds, onMarkAsRead }: Props = $props();
</script>

{#if feeds.length === 0}
	<!-- Empty state -->
{:else}
<div
	class="flex flex-col gap-4 mt-2"
	data-testid="virtual-feed-list"
	style="content-visibility: auto; contain-intrinsic-size: 800px;"
>
	{#each feeds as feed, index (feed.link)}
		<div data-testid="virtual-feed-item-{index}">
			<FeedCard
				{feed}
				isReadStatus={readFeeds.has(feed.normalizedUrl)}
				setIsReadStatus={(feedLink: string) => onMarkAsRead(feedLink)}
			/>
		</div>
	{/each}
</div>
{/if}


<script lang="ts">
import { SquareArrowOutUpRight } from "@lucide/svelte";
import { Button } from "$lib/components/ui/button";
import type { RenderFeed } from "$lib/schema/feed";

interface Props {
	feed: RenderFeed;
	isReadStatus: boolean;
	setIsReadStatus: () => void;
}

const { feed, isReadStatus, setIsReadStatus }: Props = $props();
</script>

{#if !isReadStatus}
<!-- Gradient border container with hover effects -->
<div
	class="p-[2px] rounded-[18px] border-2 border-border transition-transform duration-300 ease-in-out cursor-pointer hover:scale-[1.02] hover:shadow-lg"
	data-testid="feed-card-container"
>
	<article
		class="glass w-full p-4 rounded-2xl"
		data-testid="feed-card"
		aria-label="Feed: {feed.title}"
		style="background: var(--surface-bg);"
	>
		<div class="flex flex-col gap-2">
			<!-- Title as link -->
			<div class="flex flex-row items-center gap-2">
				<div
					class="flex items-center justify-center w-6 h-6 flex-shrink-0"
					data-testid="feed-link-icon-{feed.id}"
				>
					<SquareArrowOutUpRight
						size={16}
						style="color: var(--alt-primary);"
					/>
				</div>
				<a
					href={feed.normalizedUrl}
					target="_blank"
					rel="noopener noreferrer"
					aria-label="Open {feed.title} in external link"
					class="text-sm font-semibold text-primary hover:underline leading-tight break-words"
				>
					{feed.title}
				</a>
			</div>

			<!-- Description - Use server-generated excerpt if available -->
			<p
				class="text-xs text-foreground leading-normal break-words line-clamp-3"
			>
				{feed.excerpt}
			</p>

			<!-- Author name (if available) -->
			{#if feed.author}
				<p class="text-xs text-muted-foreground italic">by {feed.author}</p>
			{/if}

			<!-- Bottom section with button and details -->
			<div class="flex justify-between items-center mt-3 gap-3">
				<Button
					class="flex-1 text-sm font-bold px-4 min-h-[44px] border border-white/20 rounded-full"
					style="background: var(--alt-primary); color: var(--text-primary);"
					onclick={setIsReadStatus}
					aria-label="Mark {feed.title} as read"
				>
					Mark as read
				</Button>
			</div>
		</div>
	</article>
</div>
{/if}


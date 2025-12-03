<script lang="ts">
import { SquareArrowOutUpRight } from "@lucide/svelte";
import { Button } from "$lib/components/ui/button";
import type { RenderFeed } from "$lib/schema/feed";
import FeedDetails from "./FeedDetails.svelte";

interface Props {
	feed: RenderFeed;
	isReadStatus: boolean;
	setIsReadStatus: (feedLink: string) => void;
}

const { feed, isReadStatus, setIsReadStatus }: Props = $props();

const handleReadStatus = () => {
	// API呼び出しは親コンポーネントで行うため、コールバックのみ実行
	// normalizedUrlを使用して一貫性を保つ
	setIsReadStatus(feed.normalizedUrl);
};
</script>

{#if !isReadStatus}
	<!-- Gradient border container with hover effects -->
	<div
		class="p-[2px] rounded-[18px] border-2 transition-transform duration-300 ease-in-out cursor-pointer hover:scale-[1.02] hover:shadow-lg"
		style="border-color: var(--surface-border);"
		data-testid="feed-card-container"
	>
		<div
			class="glass w-full p-4 rounded-2xl"
			data-testid="feed-card"
			role="article"
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
						class="text-sm font-semibold hover:underline leading-tight break-words"
						style="color: var(--accent-primary);"
					>
						{feed.title}
					</a>
				</div>

				<!-- Description - Use server-generated excerpt if available -->
				<p
					class="text-xs leading-normal break-words line-clamp-3"
					style="color: var(--text-primary);"
				>
					{feed.excerpt}
				</p>

				<!-- Author name (if available) -->
				{#if feed.author}
					<p class="text-xs italic" style="color: var(--text-secondary);">
						by {feed.author}
					</p>
				{/if}

				<!-- Bottom section with button and details -->
				<div class="flex justify-between items-center mt-3 gap-3">
					<Button
						class="flex-1 text-sm font-bold px-4 min-h-[44px] border border-white/20 rounded-full transition-all duration-200 hover:scale-105 active:scale-95"
						style="
							background: var(--alt-primary);
							color: var(--text-primary);
						"
						onclick={handleReadStatus}
						aria-label="Mark {feed.title} as read"
					>
						Mark as read
					</Button>

					<FeedDetails feedURL={feed.link} feedTitle={feed.title} />
				</div>
			</div>
		</div>
	</div>
{/if}


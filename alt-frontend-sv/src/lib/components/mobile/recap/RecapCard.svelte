<script lang="ts">
import { ChevronDown, ChevronUp, Link as LinkIcon } from "@lucide/svelte";
import { Button } from "$lib/components/ui/button";
import type { RecapGenre } from "$lib/schema/recap";

interface Props {
	genre: RecapGenre;
}

const { genre }: Props = $props();

let isExpanded = $state(false);

const handleToggle = () => {
	isExpanded = !isExpanded;
};

// 箇条書きまたはサマリーから表示用のリストを生成
const displayItems = $derived.by(() => {
	if (genre.bullets && genre.bullets.length > 0) {
		return genre.bullets;
	}
	return genre.summary.split("\n").filter((line) => line.trim().length > 0);
});

const visibleItems = $derived(
	isExpanded ? displayItems : displayItems.slice(0, 3),
);
</script>

<div
	class="p-[2px] rounded-[18px] border-2 transition-all duration-300"
	style="border-color: var(--surface-border);"
	data-testid="recap-card-{genre.genre}"
>
	<div
		class="w-full p-4 rounded-[16px] backdrop-blur-md"
		style="background: var(--surface-bg);"
		data-testid="recap-card-container"
	>
		<div class="flex flex-col gap-4">
			<!-- ヘッダー: ジャンル名・メトリクス -->
			<div class="flex justify-between items-center flex-wrap gap-2">
				<h3
					class="text-lg font-bold uppercase tracking-wider flex-1 min-w-0 break-words"
					style="color: var(--accent-primary);"
				>
					{genre.genre}
				</h3>
				<div
					class="flex gap-3 text-xs flex-shrink-0"
					style="color: var(--text-secondary);"
				>
					<span>Clusters: {genre.clusterCount}</span>
					<span>Articles: {genre.articleCount}</span>
				</div>
			</div>

			<!-- トピックChips -->
			{#if genre.topTerms.length > 0}
				<div class="flex gap-2 items-start">
					<div
						class="w-[6px] h-[6px] rounded-full mt-[6px] flex-shrink-0"
						style="background: black;"
					></div>
					<div class="flex gap-2 flex-wrap">
						{#each genre.topTerms.slice(0, 5) as term}
							<div
								class="px-3 py-1 rounded-full text-xs border"
								style="background: rgba(255, 255, 255, 0.1); color: var(--text-primary); border-color: var(--surface-border);"
							>
								{term}
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<!-- 要約プレビュー: 箇条書きの最初の3つを表示 -->
			<div class="flex flex-col gap-2">
				{#each visibleItems as bullet, idx}
					<div class="flex gap-2 items-start">
						<div
							class="w-[6px] h-[6px] rounded-full mt-[9px] flex-shrink-0"
							style="background: black;"
						></div>
						<p
							class="text-sm leading-relaxed {isExpanded ? '' : 'line-clamp-2'}"
							style="color: var(--text-primary);"
						>
							{bullet}
						</p>
					</div>
				{/each}
			</div>

			<!-- 展開ボタン -->
			<Button
				size="sm"
				onclick={handleToggle}
				class="rounded-full font-bold transition-all duration-200 hover:scale-105 active:scale-95"
				style="background: var(--alt-primary); color: var(--text-primary);"
			>
				<div class="flex items-center gap-2">
					{#if isExpanded}
						<ChevronUp size={16} />
					{:else}
						<ChevronDown size={16} />
					{/if}
					<span>{isExpanded ? "Collapse" : "View details"}</span>
				</div>
			</Button>

			<!-- 展開時: Evidence -->
			{#if isExpanded && genre.evidenceLinks.length > 0}
				<div class="flex flex-col gap-3 pt-2 border-t"
					style="border-color: var(--surface-border);"
				>
					<!-- Evidence Links -->
					<div>
						<p
							class="text-xs font-bold mb-3 uppercase tracking-wider"
							style="color: var(--text-secondary);"
						>
							Evidence ({genre.evidenceLinks.length} articles)
						</p>
						<div class="flex flex-col gap-2">
							{#each genre.evidenceLinks as evidence}
								<div class="flex gap-2 items-start">
									<div
										class="w-[6px] h-[6px] rounded-full mt-[6px] flex-shrink-0"
										style="background: var(--alt-primary);"
									></div>
									<a
										href={evidence.sourceUrl}
										target="_blank"
										rel="noopener noreferrer"
										class="flex-1 p-2 rounded-lg border flex items-start gap-2 transition-all duration-200 hover:brightness-110"
										style="background: rgba(255, 255, 255, 0.05); border-color: var(--surface-border); color: var(--text-primary);"
										onmouseenter={(e) => {
											e.currentTarget.style.background = "rgba(255, 255, 255, 0.1)";
											e.currentTarget.style.borderColor = "var(--alt-primary)";
										}}
										onmouseleave={(e) => {
											e.currentTarget.style.background = "rgba(255, 255, 255, 0.05)";
											e.currentTarget.style.borderColor = "var(--surface-border)";
										}}
									>
										<LinkIcon size={14} class="mt-0.5 flex-shrink-0" style="color: var(--alt-primary);" />
										<p
											class="text-xs flex-1 break-words"
											style="color: var(--text-primary);"
										>
											{evidence.title}
										</p>
									</a>
								</div>
							{/each}
						</div>
					</div>
				</div>
			{/if}
		</div>
	</div>
</div>


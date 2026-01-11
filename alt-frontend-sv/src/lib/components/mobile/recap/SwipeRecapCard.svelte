<script lang="ts">
import { ChevronDown, ChevronUp, Link as LinkIcon } from "@lucide/svelte";
import { Spring } from "svelte/motion";
import { type SwipeDirection, swipe } from "$lib/actions/swipe";
import { Button } from "$lib/components/ui/button";
import type { RecapGenre } from "$lib/schema/recap";

interface Props {
	genre: RecapGenre;
	onDismiss: (direction: number) => Promise<void> | void;
	isBusy?: boolean;
}

const { genre, onDismiss, isBusy = false }: Props = $props();

// Swipe state with Spring
const SWIPE_THRESHOLD = 60;
let x = new Spring(0, { stiffness: 0.18, damping: 0.85 });
let isDragging = $state(false);
let hasSwiped = $state(false);
let swipeElement: HTMLDivElement | null = $state(null);
let scrollAreaRef: HTMLDivElement | null = $state(null);

// Derived styles
const cardStyle = $derived.by(() => {
	const translate = x.current;
	const opacity = Math.max(0.4, 1 - Math.abs(translate) / 500);

	return [
		"max-width: calc(100% - 1rem)",
		`transform: translate3d(${translate}px, 0, 0)`,
		`opacity: ${opacity}`,
		"will-change: transform, opacity",
	].join("; ");
});

// State
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

// Set up swipe event listeners reactively
$effect(() => {
	if (!swipeElement) return;

	const swipeHandler = (event: Event) => {
		if (hasSwiped) return;
		handleSwipe(event as CustomEvent<{ direction: SwipeDirection }>);
	};

	const swipeMoveHandler = (event: Event) => {
		const moveEvent = event as CustomEvent<{
			deltaX: number;
			deltaY: number;
		}>;
		const { deltaX, deltaY } = moveEvent.detail;

		// 横方向の動きが優勢なときだけ追従させる
		if (Math.abs(deltaX) > Math.abs(deltaY)) {
			isDragging = true;
			x.set(deltaX, { instant: true });
		}
	};

	const swipeEndHandler = (event: Event) => {
		x.target = 0;
		isDragging = false;
	};

	swipeElement.addEventListener("swipe", swipeHandler);
	swipeElement.addEventListener("swipe:move", swipeMoveHandler);
	swipeElement.addEventListener("swipe:end", swipeEndHandler);

	return () => {
		swipeElement?.removeEventListener("swipe", swipeHandler);
		swipeElement?.removeEventListener("swipe:move", swipeMoveHandler);
		swipeElement?.removeEventListener("swipe:end", swipeEndHandler);
	};
});

async function handleSwipe(event: CustomEvent<{ direction: SwipeDirection }>) {
	const dir = event.detail.direction;
	if (dir !== "left" && dir !== "right") return;

	hasSwiped = true;
	isDragging = false;

	const width = swipeElement?.clientWidth ?? window.innerWidth;
	const target = dir === "left" ? -width : width;

	// 画面外までスプリングで飛ばす（慣性付きで気持ちよく）
	await x.set(target, { preserveMomentum: 120 });

	// ここで「次のカードへ」「前のカードへ」のロジックを呼ぶ
	await onDismiss(dir === "left" ? -1 : 1);

	// 次のカードに備えてリセット
	hasSwiped = false;
	await x.set(0, { instant: true });
}
</script>

<div
	bind:this={swipeElement}
	class="absolute w-full h-full p-[2px] rounded-[18px] border-2 select-none"
	style="border-color: var(--surface-border); {cardStyle}; touch-action: none;"
	use:swipe={{ threshold: SWIPE_THRESHOLD, restraint: 120, allowedTime: 500 }}
	aria-busy={isBusy}
	data-testid="swipe-recap-card"
>
	<div
		class="glass w-full h-full rounded-[16px] flex flex-col"
		style="background: var(--surface-bg);"
	>
		<!-- Header -->
		<div
			class="border-b px-4 py-3 rounded-t-[16px]"
			style="border-color: var(--surface-border);"
		>
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
					<span>{genre.clusterCount} clusters</span>
					<span>{genre.articleCount} articles</span>
				</div>
			</div>
		</div>

		<!-- Scrollable Content Area -->
		<div
			bind:this={scrollAreaRef}
			style="touch-action: pan-y; overflow-x: hidden;"
			class="flex-1 overflow-y-auto overflow-x-hidden px-4 py-4 bg-transparent scroll-smooth overscroll-contain scrollbar-thin select-none"
			data-testid="recap-scroll-area"
		>
			<div class="flex flex-col gap-5">
				<!-- トピックChips -->
				{#if genre.topTerms.length > 0}
					<div class="flex gap-2 flex-wrap">
						{#each genre.topTerms.slice(0, 5) as term}
							<span
								class="px-3 py-1.5 rounded-full text-xs font-medium border transition-colors duration-200"
								style="background: var(--surface-hover); color: var(--text-primary); border-color: var(--surface-border);"
							>
								{term}
							</span>
						{/each}
					</div>
				{/if}

				<!-- 要約プレビュー: 箇条書きの最初の3つを表示 -->
				<div class="flex flex-col gap-2">
					{#each visibleItems as bullet, idx}
						<div class="flex gap-2 items-start">
							<div
								class="w-[6px] h-[6px] rounded-full mt-[9px] flex-shrink-0"
								style="background: var(--alt-primary);"
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

				<!-- 展開時: Evidence -->
				{#if isExpanded && genre.evidenceLinks.length > 0}
					<div class="flex flex-col gap-3 pt-4 border-t"
						style="border-color: var(--surface-border);"
					>
						<!-- Evidence Links -->
						<div>
							<p
								class="text-xs font-semibold mb-3 uppercase tracking-wider"
								style="color: var(--text-secondary);"
							>
								Evidence ({genre.evidenceLinks.length} articles)
							</p>
							<div class="flex flex-col gap-2">
								{#each genre.evidenceLinks as evidence}
									<a
										href={evidence.sourceUrl}
										target="_blank"
										rel="noopener noreferrer"
										class="flex items-start gap-2 p-3 rounded-xl border transition-all duration-200 hover:-translate-y-[1px]"
										style="background: var(--surface-hover); border-color: var(--surface-border); color: var(--text-primary);"
										onmouseenter={(e) => {
											e.currentTarget.style.borderColor = "var(--alt-primary)";
										}}
										onmouseleave={(e) => {
											e.currentTarget.style.borderColor = "var(--surface-border)";
										}}
									>
										<LinkIcon size={14} class="mt-0.5 flex-shrink-0" style="color: var(--alt-primary);" />
										<span
											class="text-sm flex-1 break-words leading-relaxed"
											style="color: var(--text-primary);"
										>
											{evidence.title}
										</span>
									</a>
								{/each}
							</div>
						</div>
					</div>
				{/if}
			</div>
		</div>

		<!-- Footer -->
		<div
			class="border-t px-4 py-3 rounded-b-[16px]"
			style="border-color: var(--surface-border);"
			data-testid="action-footer"
		>
			<Button
				size="sm"
				onclick={handleToggle}
				class="w-full rounded-full font-bold min-h-[44px] transition-all duration-200 hover:scale-[1.02] active:scale-95"
				style="background: var(--alt-primary); color: var(--text-primary);"
			>
				<div class="flex items-center justify-center gap-2">
					{#if isExpanded}
						<ChevronUp size={16} />
					{:else}
						<ChevronDown size={16} />
					{/if}
					<span>{isExpanded ? "Collapse" : "View details"}</span>
				</div>
			</Button>
		</div>
	</div>
</div>

<style>
	.scrollbar-thin::-webkit-scrollbar {
		width: 4px;
	}
	.scrollbar-thin::-webkit-scrollbar-track {
		background: transparent;
		border-radius: 2px;
	}
	.scrollbar-thin::-webkit-scrollbar-thumb {
		background: rgba(255, 255, 255, 0.2);
		border-radius: 2px;
	}
	.scrollbar-thin::-webkit-scrollbar-thumb:hover {
		background: rgba(255, 255, 255, 0.3);
	}

	/* Androidでテキスト選択がスワイプを妨げないように、親要素とすべての子要素でテキスト選択を無効化 */
	[data-testid="recap-scroll-area"],
	[data-testid="recap-scroll-area"] * {
		-webkit-user-select: none; /* Safari, Chrome */
		-moz-user-select: none;     /* Firefox */
		-ms-user-select: none;      /* Internet Explorer, Edge */
		user-select: none;          /* 標準 */
	}
</style>


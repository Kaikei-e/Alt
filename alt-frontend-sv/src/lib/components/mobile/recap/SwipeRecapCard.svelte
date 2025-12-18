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
		return genre.summary
			.split("\n")
			.filter((line) => line.trim().length > 0);
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

	async function handleSwipe(
		event: CustomEvent<{ direction: SwipeDirection }>,
	) {
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
	class="absolute w-full h-full bg-[var(--alt-glass)] text-[var(--alt-text-primary)] border-2 border-[var(--alt-glass-border)] shadow-[0_12px_40px_rgba(0,0,0,0.3),0_0_0_1px_rgba(255,255,255,0.1)] rounded-2xl p-4 backdrop-blur-[20px] select-none"
	use:swipe={{ threshold: SWIPE_THRESHOLD, restraint: 120, allowedTime: 500 }}
	aria-busy={isBusy}
	data-testid="swipe-recap-card"
	style={`${cardStyle}; touch-action: none;`}
>
	<div class="flex flex-col gap-0 h-full">
		<!-- Header -->
		<div
			class="relative z-[2] bg-[rgba(255,255,255,0.03)] backdrop-blur-[20px] border-b border-[var(--alt-glass-border)] px-2 py-2 rounded-t-2xl"
		>
			<p
				class="text-xs uppercase tracking-[0.08em] font-semibold"
				style="color: black;"
			>
				Swipe to navigate
			</p>
			<!-- ジャンル名・メトリクス -->
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
		</div>

		<!-- Scrollable Content Area -->
		<div
			bind:this={scrollAreaRef}
			style="touch-action: pan-y; overflow-x: hidden;"
			class="flex-1 overflow-y-auto overflow-x-hidden px-2 py-2 bg-transparent scroll-smooth overscroll-contain scrollbar-thin select-none"
			data-testid="recap-scroll-area"
		>
			<div class="flex flex-col gap-4">
				<!-- トピックChips -->
				{#if genre.topTerms.length > 0}
					<div class="flex gap-2 items-start">
						<div
							class="w-[6px] h-[6px] rounded-full mt-[6px] flex-shrink-0"
							style="background: var(--alt-primary);"
						></div>
						<div class="flex gap-2 flex-wrap">
							{#each genre.topTerms.slice(0, 5) as term}
								<div
									class="px-3 py-1 rounded-full text-xs border"
									style="background: rgba(255, 255, 255, 0.1); color: var(--text-primary); border-color: var(--alt-glass-border);"
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
					<div class="flex flex-col gap-3 pt-2 border-t"
						style="border-color: var(--alt-glass-border);"
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
											style="background: rgba(255, 255, 255, 0.05); border-color: var(--alt-glass-border); color: var(--text-primary);"
											onmouseenter={(e) => {
												e.currentTarget.style.background = "rgba(255, 255, 255, 0.1)";
												e.currentTarget.style.borderColor = "var(--alt-primary)";
											}}
											onmouseleave={(e) => {
												e.currentTarget.style.background = "rgba(255, 255, 255, 0.05)";
												e.currentTarget.style.borderColor = "var(--alt-glass-border)";
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

		<!-- Footer -->
		<div
			class="relative z-[2] bg-[rgba(0,0,0,0.25)] backdrop-blur-[20px] border-t border-[var(--alt-glass-border)] px-3 py-3 rounded-b-2xl shadow-[0_-4px_20px_rgba(0,0,0,0.3)]"
			data-testid="action-footer"
		>
			<Button
				size="sm"
				onclick={handleToggle}
				class="w-full rounded-xl font-bold text-white hover:brightness-110 active:translate-y-0 transition-all duration-200 shadow-lg bg-[slate-200] shadow-[var(--alt-primary)]/50"
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


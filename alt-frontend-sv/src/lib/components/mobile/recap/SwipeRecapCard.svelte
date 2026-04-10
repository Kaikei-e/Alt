<script lang="ts">
import { ChevronDown, ChevronUp, Link as LinkIcon } from "@lucide/svelte";
import { Spring } from "svelte/motion";
import { type SwipeDirection, swipe } from "$lib/actions/swipe";
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

		if (Math.abs(deltaX) > Math.abs(deltaY)) {
			isDragging = true;
			x.set(deltaX, { instant: true });
		}
	};

	const swipeEndHandler = (_event: Event) => {
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

	await x.set(target, { preserveMomentum: 120 });
	await onDismiss(dir === "left" ? -1 : 1);

	hasSwiped = false;
	await x.set(0, { instant: true });
}
</script>

<div
	bind:this={swipeElement}
	class="recap-card"
	style="max-width: calc(100% - 1rem); {cardStyle}; touch-action: none;"
	use:swipe={{ threshold: SWIPE_THRESHOLD, restraint: 120, allowedTime: 500 }}
	aria-busy={isBusy}
	data-testid="swipe-recap-card"
>
	<div class="card-inner">
		<!-- Header -->
		<header class="card-header">
			<div class="flex justify-between items-center flex-wrap gap-2">
				<h3 class="genre-title">
					{genre.genre}
				</h3>
				<div class="genre-meta">
					<span>{genre.clusterCount} clusters</span>
					<span>{genre.articleCount} articles</span>
				</div>
			</div>
		</header>

		<!-- Scrollable Content Area -->
		<div
			bind:this={scrollAreaRef}
			style="touch-action: pan-y; overflow-x: hidden;"
			class="scroll-area"
			data-testid="recap-scroll-area"
		>
			<div class="flex flex-col gap-5">
				<!-- Topic chips -->
				{#if genre.topTerms.length > 0}
					<div class="flex gap-2 flex-wrap">
						{#each genre.topTerms.slice(0, 5) as term}
							<span class="topic-chip">{term}</span>
						{/each}
					</div>
				{/if}

				<!-- Bullet list -->
				<div class="bullet-list">
					{#each visibleItems as bullet, idx}
						<div class="bullet-item">
							<div class="bullet-border" aria-hidden="true"></div>
							<p class="bullet-text" class:bullet-text--clamped={!isExpanded}>
								{bullet}
							</p>
						</div>
					{/each}
				</div>

				<!-- Evidence (expanded) -->
				{#if isExpanded && genre.evidenceLinks.length > 0}
					<div class="evidence-section">
						<p class="section-label">
							Evidence ({genre.evidenceLinks.length} articles)
						</p>
						<div class="flex flex-col gap-2">
							{#each genre.evidenceLinks as evidence}
								<a
									href={evidence.sourceUrl}
									target="_blank"
									rel="noopener noreferrer"
									class="evidence-link"
								>
									<LinkIcon size={14} class="evidence-icon" />
									<span class="evidence-title">
										{evidence.title}
									</span>
								</a>
							{/each}
						</div>
					</div>
				{/if}
			</div>
		</div>

		<!-- Footer -->
		<footer class="card-footer" data-testid="action-footer">
			<button
				type="button"
				onclick={handleToggle}
				class="action-btn"
			>
				<div class="flex items-center justify-center gap-2">
					{#if isExpanded}
						<ChevronUp size={16} />
					{:else}
						<ChevronDown size={16} />
					{/if}
					<span>{isExpanded ? "Collapse" : "View details"}</span>
				</div>
			</button>
		</footer>
	</div>
</div>

<style>
	.recap-card {
		position: absolute;
		width: 100%;
		height: 100%;
		user-select: none;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
	}

	.card-inner {
		width: 100%;
		height: 100%;
		display: flex;
		flex-direction: column;
	}

	/* ── Header ── */
	.card-header {
		border-bottom: 1px solid var(--surface-border);
		padding: 0.75rem 1rem;
	}

	.genre-title {
		font-family: var(--font-display);
		font-size: 1.1rem;
		font-weight: 700;
		color: var(--alt-primary);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		margin: 0;
		flex: 1;
		min-width: 0;
		word-break: break-word;
	}

	.genre-meta {
		display: flex;
		gap: 0.75rem;
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		flex-shrink: 0;
	}

	/* ── Scroll area ── */
	.scroll-area {
		flex: 1;
		overflow-y: auto;
		overflow-x: hidden;
		padding: 1rem;
		background: transparent;
		scroll-behavior: smooth;
		overscroll-behavior: contain;
		user-select: none;
	}

	.scroll-area::-webkit-scrollbar { width: 3px; }
	.scroll-area::-webkit-scrollbar-track { background: transparent; }
	.scroll-area::-webkit-scrollbar-thumb { background: var(--surface-border); }

	.scroll-area,
	.scroll-area :global(*) {
		-webkit-user-select: none;
		-moz-user-select: none;
		-ms-user-select: none;
		user-select: none;
	}

	/* ── Topic chips ── */
	.topic-chip {
		font-family: var(--font-body);
		font-size: 0.7rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1px solid var(--surface-border);
		padding: 0.3rem 0.6rem;
		transition: background 0.15s, color 0.15s, border-color 0.15s;
	}

	/* ── Bullet list ── */
	.bullet-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.bullet-item {
		display: flex;
		gap: 0.5rem;
		align-items: stretch;
	}

	.bullet-border {
		width: 1px;
		flex-shrink: 0;
		background: var(--alt-primary);
	}

	.bullet-text {
		font-family: var(--font-body);
		font-size: 0.88rem;
		line-height: 1.6;
		color: var(--alt-charcoal);
		margin: 0;
		padding: 0.15rem 0;
	}

	.bullet-text--clamped {
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	/* ── Evidence ── */
	.evidence-section {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		padding-top: 1rem;
		border-top: 1px solid var(--surface-border);
	}

	.section-label {
		font-family: var(--font-body);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0;
	}

	.evidence-link {
		display: flex;
		align-items: flex-start;
		gap: 0.5rem;
		padding: 0.6rem 0.75rem;
		border: 1px solid var(--surface-border);
		text-decoration: none;
		transition: border-color 0.15s;
	}

	.evidence-link:hover {
		border-color: var(--alt-primary);
	}

	.evidence-link :global(.evidence-icon) {
		color: var(--alt-primary);
		flex-shrink: 0;
		margin-top: 0.15rem;
	}

	.evidence-title {
		font-family: var(--font-body);
		font-size: 0.85rem;
		line-height: 1.5;
		color: var(--alt-charcoal);
		flex: 1;
		word-break: break-word;
	}

	/* ── Footer ── */
	.card-footer {
		border-top: 1px solid var(--surface-border);
		padding: 0.75rem 1rem;
	}

	.action-btn {
		width: 100%;
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
		padding: 0.5rem 1rem;
		min-height: 44px;
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.action-btn:active {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}
</style>

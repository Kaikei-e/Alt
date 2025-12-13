<script lang="ts">
  import {
    BookOpen,
    Loader,
    Sparkles,
    SquareArrowOutUpRight,
  } from "@lucide/svelte";
  import { onMount } from "svelte";
  import { Spring } from "svelte/motion";
  import { fade } from "svelte/transition";
  import { type SwipeDirection, swipe } from "$lib/actions/swipe";
  import {
    getFeedContentOnTheFlyClient,
    summarizeArticleClient,
  } from "$lib/api/client";
  import { Button } from "$lib/components/ui/button";
  import type { RenderFeed } from "$lib/schema/feed";

  interface Props {
    feed: RenderFeed;
    statusMessage: string | null;
    onDismiss: (direction: number) => Promise<void> | void;
    getCachedContent?: (feedUrl: string) => string | null;
    isBusy?: boolean;
    initialArticleContent?: string | null;
  }

  const {
    feed,
    statusMessage,
    onDismiss,
    getCachedContent,
    isBusy = false,
    initialArticleContent,
  }: Props = $props();

  // State
  let isAISummaryRequested = $state(false);
  let aiSummary = $state<string | null>(null);
  let summaryError = $state<string | null>(null);
  let isSummarizing = $state(false);

  let isContentExpanded = $state(false);
  let fullContent = $state<string | null>(null);
  let isLoadingContent = $state(false);
  let contentError = $state<string | null>(null);

  // Swipe state with Spring
  const SWIPE_THRESHOLD = 80;
  const HORIZONTAL_SWIPE_THRESHOLD = 15; // 横スワイプ検出の閾値（px）
  let x = new Spring(0, { stiffness: 0.18, damping: 0.85 });
  let isDragging = $state(false);
  let hasSwiped = $state(false);
  let swipeElement: HTMLDivElement | null = $state(null);
  let scrollAreaRef: HTMLDivElement | null = $state(null);
  let isHorizontalSwipeActive = $state(false);

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

  // Derived
  const sanitizedFullContent = $derived(fullContent);
  const hasDescription = $derived(Boolean(feed.description));
  const publishedLabel = $derived.by(() => {
    if (feed.created_at) {
      try {
        return new Date(feed.created_at).toLocaleString();
      } catch {
        // Fallback
      }
    }
    if (!feed.published) return null;
    try {
      return new Date(feed.published).toLocaleString();
    } catch {
      return feed.published;
    }
  });

  // Auto-fetch content
  onMount(() => {
    // Initialize with prop value if available
    if (initialArticleContent) {
      fullContent = initialArticleContent;
    }

    const cached = getCachedContent?.(feed.link);
    if (cached) {
      fullContent = cached;
    } else if (!fullContent) {
      // Background fetch
      getFeedContentOnTheFlyClient(feed.link)
        .then((res) => {
          if (res.content) {
            fullContent = res.content;
          }
        })
        .catch((err) => {
          console.error("[SwipeFeedCard] Error auto-fetching content:", err);
        });
    }
  });

  // Set up swipe event listeners reactively
  $effect(() => {
    if (!swipeElement) return;

    const swipeHandler = (event: Event) => {
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

        // 横方向の動きが閾値を超えたら、スクロールを無効化してカードのスワイプを優先
        if (Math.abs(deltaX) >= HORIZONTAL_SWIPE_THRESHOLD && scrollAreaRef) {
          if (!isHorizontalSwipeActive) {
            isHorizontalSwipeActive = true;
            scrollAreaRef.style.touchAction = "none";
          }
        }
      }
    };

    const swipeEndHandler = (event: Event) => {
      // ドラッグが終わったので中央に戻す
      // 実際にスワイプが成立した場合は、swipe イベント → handleSwipe → onDismiss が走るので、
      // カード自体はすぐ差し替えられる
      // 成立しなかった場合だけ「中央にスナップバック」という役割分担
      x.target = 0;
      isDragging = false;

      // スワイプが成立しなかった場合、touch-action をリセット
      // スワイプが成立した場合は handleSwipe で処理されるため、そのまま維持
      if (isHorizontalSwipeActive && scrollAreaRef && !hasSwiped) {
        isHorizontalSwipeActive = false;
        scrollAreaRef.style.touchAction = "pan-y";
      }
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

  async function handleToggleContent() {
    if (!isContentExpanded && !fullContent) {
      const cached = getCachedContent?.(feed.link);
      if (cached) {
        fullContent = cached;
        isContentExpanded = true;
        return;
      }

      isLoadingContent = true;
      contentError = null;

      try {
        const res = await getFeedContentOnTheFlyClient(feed.link);
        if (res.content) {
          fullContent = res.content;
        } else {
          contentError = "Could not fetch article content";
        }
      } catch (err) {
        console.error("Error fetching content:", err);
        contentError = "Could not fetch article content";
      } finally {
        isLoadingContent = false;
      }
    }
    isContentExpanded = !isContentExpanded;
  }

  async function handleGenerateAISummary() {
    // Hide existing SUMMARY section
    isAISummaryRequested = true;
    isSummarizing = true;
    summaryError = null;

    try {
      const res = await summarizeArticleClient(feed.link);
      if (res.success && res.summary) {
        aiSummary = res.summary;
      } else {
        summaryError = "Failed to generate the summary";
      }
    } catch (err) {
      console.error("Error summarizing article:", err);
      summaryError = "Failed to generate the summary";
    } finally {
      isSummarizing = false;
    }
  }

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

    // ここで「次の記事へ」「前の記事へ」のロジックを呼ぶ
    await onDismiss(dir === "left" ? -1 : 1);

    // 次のカードに備えてリセット
    hasSwiped = false;
    await x.set(0, { instant: true });

    // touch-action もリセット
    if (isHorizontalSwipeActive && scrollAreaRef) {
      isHorizontalSwipeActive = false;
      scrollAreaRef.style.touchAction = "pan-y";
    }
  }
</script>

<div
  bind:this={swipeElement}
  class="absolute w-full h-[95dvh] bg-[var(--alt-glass)] text-[var(--alt-text-primary)] border-2 border-[var(--alt-glass-border)] shadow-[0_12px_40px_rgba(0,0,0,0.3),0_0_0_1px_rgba(255,255,255,0.1)] rounded-2xl p-4 backdrop-blur-[20px] select-none"
  use:swipe={{ threshold: SWIPE_THRESHOLD, restraint: 120, allowedTime: 500 }}
  aria-busy={isBusy}
  data-testid="swipe-card"
  style={`${cardStyle}; touch-action: none;`}
>
  <div class="flex flex-col gap-0 h-full">
    <!-- Header -->
    <div
      class="relative z-[2] bg-[rgba(255,255,255,0.03)] backdrop-blur-[20px] border-b border-[var(--alt-glass-border)] px-2 py-2 rounded-t-2xl"
    >
      <p
        class="text-sm mb-2 uppercase tracking-[0.08em] font-semibold"
        style="color: black;"
      >
        Swipe to mark as read
      </p>
      <div class="flex items-center gap-2">
        <a
          href={feed.link}
          target="_blank"
          rel="noopener noreferrer"
          aria-label="Open article in new tab"
          class="flex items-center justify-center text-[var(--alt-text-primary)] border border-[var(--alt-glass-border)] rounded-md p-2 flex-1 min-w-0 hover:bg-[rgba(255,255,255,0.05)] hover:border-[var(--alt-primary)] transition-colors"
        >
          <div class="shrink-0 mr-2">
            <SquareArrowOutUpRight
              class="text-[var(--alt-primary)]"
              size={20}
            />
          </div>
          <h2
            class="text-xl font-bold flex-1 break-words whitespace-normal min-w-0"
          >
            {feed.title}
          </h2>
        </a>
      </div>
      {#if publishedLabel}
        <p class="text-[var(--alt-text-secondary)] text-sm mt-2">
          {publishedLabel}
        </p>
      {/if}
    </div>

    <!-- Only Vertical Scroll Area -->
    <div
      bind:this={scrollAreaRef}
      style="touch-action: pan-y; overflow-x: hidden;"
      class="flex-1 overflow-y-auto overflow-x-hidden px-2 py-2 bg-transparent scroll-smooth overscroll-contain scrollbar-thin"
      data-testid="unified-scroll-area"
    >
      {#if hasDescription && !isAISummaryRequested}
        <div class="mb-4 overflow-x-hidden" transition:fade>
          <p
            class="text-xs text-[var(--alt-text-secondary)] font-bold mb-2 uppercase tracking-widest"
          >
            Summary
          </p>
          <div
            class="text-sm text-[var(--alt-text-primary)] leading-[1.7] break-words overflow-wrap-anywhere"
          >
            {@html feed.description}
          </div>
        </div>
      {/if}

      {#if isAISummaryRequested}
        <div
          class="px-4 pt-2 pb-4 border-t mb-4 overflow-x-hidden"
          data-testid="ai-summary-section"
          transition:fade
        >
          <p
            class="text-xs text-[var(--alt-text-secondary)] font-semibold mb-2 uppercase tracking-[0.18em]"
          >
            {isSummarizing ? "SUMMARY" : "AI SUMMARY"}
          </p>
          {#if isSummarizing}
            <div class="flex flex-col items-center gap-3 py-4">
              <Loader
                class="animate-spin text-[var(--alt-primary)]"
                size={20}
              />
              <span class="text-[var(--alt-text-secondary)] text-sm"
                >Now summarizing ....</span
              >
            </div>
          {:else if summaryError}
            <p
              class="text-[var(--alt-text-secondary)] text-sm text-center py-4"
            >
              {summaryError}
            </p>
          {:else if aiSummary}
            <p
              class="text-sm text-[var(--alt-text-primary)] leading-relaxed whitespace-pre-wrap break-words overflow-wrap-anywhere"
            >
              {aiSummary}
            </p>
          {/if}
        </div>
      {/if}

      {#if isContentExpanded}
        <div
          class="mb-4 p-4 bg-[rgba(255,255,255,0.03)] rounded-xl border border-[var(--alt-glass-border)] overflow-x-hidden"
          data-testid="content-section"
          transition:fade
        >
          <p
            class="text-xs text-[var(--alt-text-secondary)] font-bold mb-2 uppercase tracking-widest"
          >
            Full Article
          </p>
          {#if isLoadingContent}
            <div class="flex justify-center py-4 gap-2">
              <Loader
                class="animate-spin text-[var(--alt-primary)]"
                size={20}
              />
              <span class="text-[var(--alt-text-secondary)] text-sm"
                >Loading article content...</span
              >
            </div>
          {:else if contentError}
            <p class="text-[var(--alt-text-secondary)] text-sm text-center">
              {contentError}
            </p>
          {:else if sanitizedFullContent}
            <div
              class="text-sm text-[var(--alt-text-primary)] leading-[1.7] prose prose-invert max-w-none break-words overflow-wrap-anywhere overflow-x-hidden"
            >
              {@html sanitizedFullContent}
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Footer -->
    <div
      class="relative z-[2] bg-[rgba(0,0,0,0.25)] backdrop-blur-[20px] border-t border-[var(--alt-glass-border)] px-3 py-3 rounded-b-2xl shadow-[0_-4px_20px_rgba(0,0,0,0.3)]"
      data-testid="action-footer"
    >
      <div class="flex gap-2 w-full justify-between">
        <Button
          onclick={handleToggleContent}
          size="sm"
          class="flex-1 rounded-xl font-bold text-white hover:brightness-110 active:translate-y-0 transition-all duration-200 shadow-lg {isContentExpanded
            ? 'bg-[slate-200] shadow-[var(--alt-secondary)]/50'
            : 'bg-[slate-200] shadow-[var(--alt-primary)]/50'}"
          disabled={isLoadingContent}
        >
          <BookOpen class="mr-2 h-4 w-4" />
          {isLoadingContent
            ? "Loading..."
            : isContentExpanded
              ? "Hide"
              : "Article"}
        </Button>
        <Button
          onclick={handleGenerateAISummary}
          size="sm"
          class="flex-1 rounded-xl font-bold text-white hover:brightness-110 active:translate-y-0 transition-all duration-200 shadow-lg {isAISummaryRequested
            ? 'bg-[slate-200] shadow-[var(--alt-secondary)]/50'
            : 'bg-[slate-200] shadow-[var(--alt-primary)]/50'}"
          disabled={isSummarizing}
        >
          <Sparkles class="mr-2 h-4 w-4" />
          {isSummarizing
            ? "Summarizing..."
            : isAISummaryRequested
              ? "Summary"
              : "Summary"}
        </Button>
      </div>
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
</style>

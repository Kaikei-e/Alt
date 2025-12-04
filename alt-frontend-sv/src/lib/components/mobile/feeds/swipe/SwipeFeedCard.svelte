<script lang="ts">
  import { swipe, type SwipeDirection } from "$lib/actions/swipe";
  import type { RenderFeed } from "$lib/schema/feed";
  import {
    BookOpen,
    Sparkles,
    SquareArrowOutUpRight,
    Loader2,
  } from "@lucide/svelte";
  import { Button } from "$lib/components/ui/button";
  import {
    getFeedContentOnTheFlyClient,
    getArticleSummaryClient,
    summarizeArticleClient,
  } from "$lib/api/client";
  import { onMount, onDestroy } from "svelte";
  import { fade } from "svelte/transition";

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
  let isSummaryExpanded = $state(false);
  let summary = $state<string | null>(null);
  let isLoadingSummary = $state(false);
  let summaryError = $state<string | null>(null);
  let isSummarizing = $state(false);

  let isContentExpanded = $state(false);
  let fullContent = $state<string | null>(null);
  let isLoadingContent = $state(false);
  let contentError = $state<string | null>(null);

  // Swipe state
  let translateX = $state(0);
  let isDragging = $state(false);
  let swipeElement: HTMLDivElement | null = $state(null);

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

    if (fullContent) {
      // Still need to set up swipe listener even if content is already loaded
      if (swipeElement) {
        const swipeHandler = (event: Event) => {
          handleSwipe(event as CustomEvent<{ direction: SwipeDirection }>);
        };
        swipeElement.addEventListener("swipe", swipeHandler);
        return () => {
          swipeElement?.removeEventListener("swipe", swipeHandler);
        };
      }
      return;
    }

    const cached = getCachedContent?.(feed.link);
    if (cached) {
      fullContent = cached;
    } else {
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

    // Add swipe event listener
    if (swipeElement) {
      const swipeHandler = (event: Event) => {
        handleSwipe(event as CustomEvent<{ direction: SwipeDirection }>);
      };
      swipeElement.addEventListener("swipe", swipeHandler);
      return () => {
        swipeElement?.removeEventListener("swipe", swipeHandler);
      };
    }
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

  async function fetchSummary() {
    isLoadingSummary = true;
    summaryError = null;
    try {
      const res = await getArticleSummaryClient(feed.link);
      if (res.matched_articles && res.matched_articles.length > 0) {
        summary = res.matched_articles[0].content;
      } else {
        summaryError = "Could not fetch summary";
      }
    } catch (err) {
      console.error("Error fetching summary:", err);
      summaryError = "Could not fetch summary";
    } finally {
      isLoadingSummary = false;
    }
  }

  async function handleToggleSummary() {
    if (!isSummaryExpanded && !summary) {
      await fetchSummary();
    }
    isSummaryExpanded = !isSummaryExpanded;
  }

  async function handleSummarizeNow() {
    isSummarizing = true;
    summaryError = null;
    try {
      const res = await summarizeArticleClient(feed.link);
      if (res.success && res.summary) {
        summary = res.summary;
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

  function handleSwipe(event: CustomEvent<{ direction: SwipeDirection }>) {
    const dir = event.detail.direction;
    if (dir === "left") {
      void onDismiss(-1);
    } else if (dir === "right") {
      void onDismiss(1);
    }
  }
</script>

<div
  bind:this={swipeElement}
  class="absolute w-full max-w-[30rem] h-[95dvh] bg-[var(--alt-glass)] text-[var(--alt-text-primary)] border-2 border-[var(--alt-glass-border)] shadow-[0_12px_40px_rgba(0,0,0,0.3),0_0_0_1px_rgba(255,255,255,0.1)] rounded-2xl p-4 backdrop-blur-[20px] touch-none select-none"
  use:swipe={{ threshold: 80, restraint: 100, allowedTime: 500 }}
  aria-busy={isBusy}
  data-testid="swipe-card"
>
  <div class="flex flex-col gap-0 h-full">
    <!-- Header -->
    <div
      class="relative z-[2] bg-[rgba(255,255,255,0.03)] backdrop-blur-[20px] border-b border-[var(--alt-glass-border)] px-2 py-2 rounded-t-2xl"
    >
      <p
        class="text-sm text-[var(--alt-text-secondary)] mb-2 uppercase tracking-[0.08em] font-semibold bg-clip-text text-transparent bg-[var(--accent-gradient)]"
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

    <!-- Scroll Area -->
    <div
      class="flex-1 overflow-auto px-2 py-2 bg-transparent scroll-smooth overscroll-contain scrollbar-thin"
      data-testid="unified-scroll-area"
    >
      {#if hasDescription}
        <div class="mb-4">
          <p
            class="text-xs text-[var(--alt-text-secondary)] font-bold mb-2 uppercase tracking-widest"
          >
            Summary
          </p>
          <p class="text-sm text-[var(--alt-text-primary)] leading-[1.7]">
            {@html feed.description}
          </p>
        </div>
      {/if}

      {#if isContentExpanded}
        <div
          class="mb-4 p-4 bg-[rgba(255,255,255,0.03)] rounded-xl border border-[var(--alt-glass-border)]"
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
              <Loader2
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
              class="text-sm text-[var(--alt-text-primary)] leading-[1.7] prose prose-invert max-w-none"
            >
              {@html sanitizedFullContent}
            </div>
          {/if}
        </div>
      {/if}

      {#if isSummaryExpanded}
        <div
          class="p-4 bg-[rgba(255,255,255,0.03)] rounded-xl border border-[var(--alt-glass-border)]"
          data-testid="summary-section"
          transition:fade
        >
          <p
            class="text-xs text-[var(--alt-text-secondary)] font-bold mb-2 uppercase tracking-widest"
          >
            Summary
          </p>
          {#if isLoadingSummary}
            <div class="flex justify-center py-4 gap-2">
              <Loader2
                class="animate-spin text-[var(--alt-primary)]"
                size={20}
              />
              <span class="text-[var(--alt-text-secondary)] text-sm"
                >Loading summary...</span
              >
            </div>
          {:else if isSummarizing}
            <div class="flex flex-col gap-3 py-4">
              <div class="flex justify-center gap-2">
                <Loader2
                  class="animate-spin text-[var(--alt-primary)]"
                  size={20}
                />
                <span class="text-[var(--alt-text-secondary)] text-sm"
                  >Generating summary...</span
                >
              </div>
              <p class="text-[var(--alt-text-secondary)] text-xs text-center">
                This may take a few seconds
              </p>
            </div>
          {:else if summaryError}
            <div class="flex flex-col gap-3 w-full">
              <p class="text-[var(--alt-text-secondary)] text-sm text-center">
                {summaryError}
              </p>
              {#if summaryError === "Could not fetch summary"}
                <div class="flex flex-col gap-2 w-full">
                  <Button
                    size="sm"
                    onclick={fetchSummary}
                    class="w-full rounded-xl bg-[var(--alt-primary)] text-white hover:bg-[var(--alt-secondary)]"
                    disabled={isLoadingSummary}
                  >
                    Retry
                  </Button>
                  <Button
                    size="sm"
                    onclick={handleSummarizeNow}
                    class="w-full rounded-xl bg-[var(--alt-primary)] text-white hover:bg-[var(--alt-secondary)]"
                    disabled={isSummarizing}
                  >
                    {#if isSummarizing}
                      <Loader2 class="mr-2 h-4 w-4 animate-spin" />
                      Generating...
                    {:else}
                      <Sparkles class="mr-2 h-4 w-4" />
                      Summarize Now
                    {/if}
                  </Button>
                </div>
              {/if}
            </div>
          {:else if summary}
            <p
              class="text-sm text-[var(--alt-text-primary)] leading-[1.7] whitespace-pre-wrap"
            >
              {summary}
            </p>
          {:else}
            <div class="flex flex-col gap-3 w-full">
              <p class="text-[var(--alt-text-secondary)] text-sm text-center">
                No summary available for this article
              </p>
              <Button
                size="sm"
                onclick={handleSummarizeNow}
                class="w-full rounded-xl bg-[var(--alt-primary)] text-white hover:bg-[var(--alt-secondary)]"
                disabled={isSummarizing}
              >
                {#if isSummarizing}
                  <Loader2 class="mr-2 h-4 w-4 animate-spin" />
                  Generating...
                {:else}
                  <Sparkles class="mr-2 h-4 w-4" />
                  Summarize Now
                {/if}
              </Button>
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Footer -->
    <div
      class="relative z-[2] bg-[rgba(255,255,255,0.05)] backdrop-blur-[20px] border-t border-[var(--alt-glass-border)] px-3 py-3 rounded-b-2xl"
      data-testid="action-footer"
    >
      <div class="flex gap-2 w-full justify-between">
        <Button
          onclick={handleToggleContent}
          size="sm"
          class="flex-1 rounded-xl font-bold text-white hover:brightness-110 active:translate-y-0 transition-all duration-200 {isContentExpanded
            ? 'bg-[var(--alt-secondary)]'
            : 'bg-[var(--alt-primary)]'}"
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
          onclick={handleToggleSummary}
          size="sm"
          class="flex-1 rounded-xl font-bold text-white hover:brightness-110 active:translate-y-0 transition-all duration-200 {isSummaryExpanded
            ? 'bg-[var(--alt-secondary)]'
            : 'bg-[var(--alt-primary)]'}"
        >
          <Sparkles class="mr-2 h-4 w-4" />
          {isSummaryExpanded ? "Hide" : "Summary"}
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

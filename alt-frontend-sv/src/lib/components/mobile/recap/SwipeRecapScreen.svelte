<script lang="ts">
import { fly } from "svelte/transition";
import { Button } from "$lib/components/ui/button";
import type { RecapGenre, RecapSummary } from "$lib/schema/recap";
import SwipeRecapCard from "./SwipeRecapCard.svelte";

interface Props {
	genres: RecapGenre[];
	summaryData?: RecapSummary | null;
}

const { genres, summaryData }: Props = $props();

// State
let activeIndex = $state(0);

// Derived
const activeGenre = $derived(genres[activeIndex]);
const nextGenre = $derived(genres[(activeIndex + 1) % genres.length]);
const prevGenre = $derived(
	genres[(activeIndex - 1 + genres.length) % genres.length],
);
const currentPosition = $derived(activeIndex + 1);
const totalCount = $derived(genres.length);

// 無限ループ機能付きスワイプハンドラー
async function handleSwipe(direction: number) {
	if (direction > 0) {
		// 右スワイプ（前のカード）
		activeIndex = activeIndex === 0 ? genres.length - 1 : activeIndex - 1;
	} else {
		// 左スワイプ（次のカード）
		activeIndex = activeIndex === genres.length - 1 ? 0 : activeIndex + 1;
	}
}
</script>

<div
  class="min-h-[100dvh] relative flex flex-col items-center overflow-hidden bg-[var(--app-bg)]"
>
  {#if genres.length === 0}
    <div class="flex flex-col items-center justify-center p-6 text-center">
      <p class="text-[var(--alt-text-secondary)] mb-4">
        No recap data available
      </p>
    </div>
  {:else if activeGenre}
    <!-- Header Info (Top) -->
    {#if summaryData}
      <div class="w-full max-w-[30rem] mx-auto px-2 pt-2 pb-2 z-20">
        <!-- Title and date container: 2 columns -->
        <div
          class="w-full flex flex-row gap-2 px-2 py-2 rounded-lg backdrop-blur-md border justify-start items-center"
          style="background: rgba(0, 0, 0, 0.1); border-color: var(--alt-glass-border);"
        >
          <h1
            class="flex-1 flex items-center justify-center text-md font-bold text-center"
            style="color: black;"
          >
            7 Days
            <br />
            Recap
          </h1>
          <div
            class="flex-1 flex flex-col gap-2 items-center justify-center text-center"
          >
            <p class="text-[10px] mb-0.5" style="color: var(--text-secondary);">
              Executed: {new Date(summaryData.executedAt).toLocaleString(
                "en-US",
              )}
            </p>
            <p class="text-[10px]" style="color: var(--text-secondary);">
              {summaryData.totalArticles.toLocaleString()} articles analyzed
            </p>
          </div>
        </div>
      </div>
    {/if}

    <!-- Position Indicator (Below Header) -->
    <div class="w-full max-w-[30rem] px-4 pb-2 z-20 flex justify-center">
      <div
        class="flex items-center gap-2 px-3 py-1.5 rounded-full backdrop-blur-md border"
        style="background: rgba(0, 0, 0, 0.3); border-color: var(--alt-glass-border);"
      >
        <!-- Counter -->
        <span class="text-xs font-semibold" style="color: var(--text-primary);">
          {currentPosition} / {totalCount}
        </span>
        <!-- Dot Indicator -->
        <div class="flex gap-1.5 items-center">
          {#each genres as _, idx}
            <div
              class="w-1.5 h-1.5 rounded-full transition-all duration-200"
              style="background: {idx === activeIndex
                ? 'var(--alt-primary)'
                : 'rgba(255, 255, 255, 0.3)'};"
              aria-label="Card {idx + 1} of {totalCount}"
            ></div>
          {/each}
        </div>
      </div>
    </div>

    <!-- Card Container -->
    <div
      class="relative w-full max-w-[30rem] flex-1 px-2 sm:px-4 overflow-hidden"
    >
      <!-- Next card (background preview) -->
      <div
        class="absolute w-full h-full bg-[var(--alt-glass)] border-2 border-[var(--alt-glass-border)] rounded-2xl p-4 opacity-50 pointer-events-none"
        aria-hidden="true"
        style="max-width: calc(100% - 1rem)"
      ></div>

      <!-- Active card -->
      {#key activeGenre.genre}
        <div
          class="absolute w-full h-full"
          in:fly={{ x: 0, y: 0, duration: 300 }}
          out:fly={{ x: 0, y: 0, duration: 300 }}
        >
          <SwipeRecapCard
            genre={activeGenre}
            onDismiss={handleSwipe}
            isBusy={false}
          />
        </div>
      {/key}
    </div>
  {/if}
</div>

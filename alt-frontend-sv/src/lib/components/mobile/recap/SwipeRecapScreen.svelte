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
      <div class="w-full max-w-[30rem] mx-auto px-4 pt-3 pb-3 z-20">
        <div
          class="flex flex-col gap-1 px-4 py-3 rounded-xl border"
          style="background: var(--surface-bg); border-color: var(--surface-border);"
        >
          <div class="flex justify-between items-center">
            <h1
              class="text-xl font-bold tracking-tight"
              style="color: var(--accent-primary);"
            >
              7 Days Recap
            </h1>
            <span
              class="text-sm font-semibold"
              style="color: var(--text-primary);"
            >
              {currentPosition} / {totalCount}
            </span>
          </div>
          <p class="text-xs" style="color: var(--text-secondary);">
            {new Date(summaryData.executedAt).toLocaleDateString("en-US", {
              month: "short",
              day: "numeric",
              hour: "2-digit",
              minute: "2-digit",
            })} · {summaryData.totalArticles.toLocaleString()} articles
          </p>
        </div>
      </div>
    {/if}

    <!-- Card Container -->
    <div
      class="relative w-full max-w-[30rem] flex-1 px-2 sm:px-4 overflow-hidden"
    >
      <!-- Next card (background preview - subtle depth indicator) -->
      <div
        class="absolute w-full h-full rounded-[18px] border opacity-30 pointer-events-none"
        aria-hidden="true"
        style="max-width: calc(100% - 1rem); background: var(--surface-bg); border-color: var(--surface-border);"
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

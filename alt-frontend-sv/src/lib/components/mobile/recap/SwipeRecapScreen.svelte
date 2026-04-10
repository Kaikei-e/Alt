<script lang="ts">
import { fly } from "svelte/transition";
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

async function handleSwipe(direction: number) {
	if (direction > 0) {
		activeIndex = activeIndex === 0 ? genres.length - 1 : activeIndex - 1;
	} else {
		activeIndex = activeIndex === genres.length - 1 ? 0 : activeIndex + 1;
	}
}
</script>

<div class="recap-screen">
  {#if genres.length === 0}
    <div class="empty-state">
      <p class="empty-text">No recap data available</p>
    </div>
  {:else if activeGenre}
    <!-- Header -->
    {#if summaryData}
      <div class="screen-header">
        <div class="header-card">
          <div class="flex justify-between items-center">
            <h1 class="header-title">7 Days Recap</h1>
            <span class="header-position">
              {currentPosition} / {totalCount}
            </span>
          </div>
          <p class="header-meta">
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
    <div class="card-container">
      <!-- Background card -->
      <div
        class="background-card"
        aria-hidden="true"
      ></div>

      <!-- Active card -->
      {#key activeGenre.genre}
        <div
          class="card-wrapper"
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

<style>
  .recap-screen {
    min-height: 100dvh;
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    overflow: hidden;
    background: var(--surface-bg);
  }

  .screen-header {
    width: 100%;
    max-width: 30rem;
    margin: 0 auto;
    padding: 0.75rem 1rem;
    z-index: 20;
  }

  .header-card {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    padding: 0.75rem 1rem;
    border: 1px solid var(--surface-border);
    background: var(--surface-bg);
  }

  .header-title {
    font-family: var(--font-display);
    font-size: 1.15rem;
    font-weight: 700;
    color: var(--alt-charcoal);
    margin: 0;
  }

  .header-position {
    font-family: var(--font-mono);
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--alt-charcoal);
  }

  .header-meta {
    font-family: var(--font-mono);
    font-size: 0.65rem;
    color: var(--alt-ash);
    margin: 0;
  }

  .card-container {
    position: relative;
    width: 100%;
    max-width: 30rem;
    flex: 1;
    padding: 0 0.5rem;
    overflow: hidden;
  }

  .background-card {
    position: absolute;
    width: 100%;
    height: 100%;
    max-width: calc(100% - 1rem);
    background: var(--surface-bg);
    border: 1px solid var(--surface-border);
    opacity: 0.3;
    pointer-events: none;
  }

  .card-wrapper {
    position: absolute;
    width: 100%;
    height: 100%;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 1.5rem;
    text-align: center;
  }

  .empty-text {
    font-family: var(--font-body);
    font-size: 0.9rem;
    color: var(--alt-slate);
    margin: 0;
  }
</style>

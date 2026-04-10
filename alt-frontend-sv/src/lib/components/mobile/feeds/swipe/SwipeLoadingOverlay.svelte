<script lang="ts">
import { fade } from "svelte/transition";

interface Props {
	isVisible: boolean;
}

const { isVisible }: Props = $props();
</script>

{#if isVisible}
  <div
    class="overlay-container"
    transition:fade={{ duration: 200 }}
    data-testid="swipe-progress-indicator"
  >
    <div class="overlay-card">
      <div class="loading-pulse" aria-hidden="true"></div>
      <p class="loading-text">Loading dispatch...</p>
      <span class="sr-only">Loading new article</span>
    </div>
  </div>
{/if}

<style>
  .overlay-container {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    padding: 0 1rem;
    padding-bottom: calc(1rem + env(safe-area-inset-bottom, 0px));
    pointer-events: none;
    z-index: 10;
  }

  .overlay-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.5rem;
    background: var(--surface-bg);
    border: 1px solid var(--surface-border);
    padding: 1rem;
    max-width: 26rem;
    margin: 0 auto;
  }

  .loading-pulse {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--alt-ash);
    animation: pulse 1.2s ease-in-out infinite;
  }

  .loading-text {
    font-family: var(--font-body);
    font-size: 0.75rem;
    font-style: italic;
    color: var(--alt-ash);
    margin: 0;
  }

  @keyframes pulse {
    0%, 100% { opacity: 0.3; }
    50% { opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .loading-pulse {
      animation: none;
      opacity: 0.6;
    }
  }
</style>

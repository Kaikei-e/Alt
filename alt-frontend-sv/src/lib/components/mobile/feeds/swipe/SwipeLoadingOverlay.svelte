<script lang="ts">
  import { fade } from "svelte/transition";

  interface Props {
    isVisible: boolean;
    reduceMotion?: boolean;
  }

  const { isVisible, reduceMotion = false }: Props = $props();
</script>

{#if isVisible}
  <div
    class="absolute left-0 right-0 bottom-0 px-4 pb-[calc(1rem+env(safe-area-inset-bottom,0px))] pointer-events-none z-10"
    transition:fade={{ duration: 200 }}
    data-testid="swipe-progress-indicator"
  >
    <div
      class="rounded-xl border border-[var(--alt-glass-border)] bg-[rgba(10,10,20,0.85)] p-4 max-w-[26rem] mx-auto shadow-[0_8px_30px_rgba(0,0,0,0.35)]"
    >
      <p class="text-xs text-[var(--alt-text-secondary)] text-center">
        Loading new article
      </p>
      <div
        class="mt-3 h-1 rounded-full bg-[rgba(255,255,255,0.12)] overflow-hidden"
        aria-hidden="true"
      >
        <div
          class="h-full w-[45%] rounded-full bg-[var(--alt-primary)] opacity-85"
          style:transform={reduceMotion ? "translateX(0)" : undefined}
          class:animate-loading-bar={!reduceMotion}
        ></div>
      </div>
      <span class="sr-only">Loading new article</span>
    </div>
  </div>
{/if}

<style>
  @keyframes loading-bar {
    0% {
      transform: translateX(-60%);
    }
    50% {
      transform: translateX(-10%);
    }
    100% {
      transform: translateX(120%);
    }
  }

  .animate-loading-bar {
    animation: loading-bar 1.4s ease-in-out infinite;
  }
</style>

<script lang="ts">
import { ArrowLeft, SlidersHorizontal, Undo2 } from "@lucide/svelte";

interface Props {
	mode: "default" | "visual-preview";
	readCount: number;
	canUndo: boolean;
	onUndo: () => void;
	onOpenFilter: () => void;
	filterActive?: boolean;
}

const {
	mode,
	readCount,
	canUndo,
	onUndo,
	onOpenFilter,
	filterActive = false,
}: Props = $props();

const kicker = $derived(
	mode === "visual-preview" ? "Photo Wire" : "Wire Dispatch",
);
</script>

<header class="dispatch-header" data-testid="dispatch-header">
  <a
    href="/menu"
    class="header-action"
    aria-label="Back to menu"
    data-testid="dispatch-back"
  >
    <ArrowLeft size={16} />
    <span class="header-action-label">Menu</span>
  </a>

  <div class="dispatch-masthead" aria-hidden="false">
    <span class="dispatch-kicker" data-testid="dispatch-kicker">{kicker}</span>
    <span class="dispatch-counter" data-testid="dispatch-counter"
      >№ {readCount}</span
    >
  </div>

  <div class="header-actions-right">
    <button
      type="button"
      class="header-action"
      disabled={!canUndo}
      onclick={onUndo}
      aria-label="Undo last read"
      data-testid="dispatch-undo"
    >
      <Undo2 size={16} />
      <span class="header-action-label">Undo</span>
    </button>
    <button
      type="button"
      class="header-action"
      onclick={onOpenFilter}
      aria-label="Filter and sort"
      data-testid="swipe-filter-trigger"
    >
      <span class="filter-icon-wrap">
        <SlidersHorizontal size={16} />
        {#if filterActive}
          <span class="filter-badge" data-testid="filter-active-badge"></span>
        {/if}
      </span>
      <span class="header-action-label">Filter</span>
    </button>
  </div>
</header>

<style>
  .dispatch-header {
    position: relative;
    display: flex;
    align-items: stretch;
    justify-content: space-between;
    width: 100%;
    padding-top: env(safe-area-inset-top, 0px);
    background: var(--surface-bg);
    border-bottom: 1px solid var(--surface-border);
    flex-shrink: 0;
  }

  .dispatch-masthead {
    position: absolute;
    left: 50%;
    top: env(safe-area-inset-top, 0px);
    transform: translateX(-50%);
    height: 48px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.05rem;
    pointer-events: none;
  }

  .dispatch-kicker {
    font-family: var(--font-mono);
    font-size: 0.65rem;
    font-weight: 600;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--alt-charcoal);
  }

  .dispatch-counter {
    font-family: var(--font-mono);
    font-size: 0.6rem;
    letter-spacing: 0.08em;
    color: var(--alt-ash);
  }

  .header-actions-right {
    display: flex;
    align-items: stretch;
  }

  .header-action {
    display: inline-flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.1rem;
    min-width: 52px;
    min-height: 48px;
    padding: 0.25rem 0.5rem;
    background: transparent;
    border: none;
    color: var(--alt-charcoal);
    text-decoration: none;
    cursor: pointer;
    touch-action: manipulation;
  }

  .header-action:active:not(:disabled) {
    background: var(--alt-charcoal);
    color: var(--surface-bg);
  }

  .header-action:disabled {
    color: var(--alt-ash);
    cursor: not-allowed;
  }

  .header-action-label {
    font-family: var(--font-mono);
    font-size: 0.55rem;
    letter-spacing: 0.1em;
    text-transform: uppercase;
  }

  .filter-icon-wrap {
    position: relative;
    display: inline-flex;
  }

  .filter-badge {
    position: absolute;
    top: -2px;
    right: -4px;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--alt-primary);
  }
</style>

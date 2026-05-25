<script lang="ts">
import { Headphones, Loader2, Pause, Play, X } from "@lucide/svelte";
import type { TtsPlaybackStore } from "$lib/stores/ttsPlayback.svelte";

interface Props {
	store: TtsPlaybackStore;
}

const { store }: Props = $props();

const playPauseLabel = $derived(store.isPlaying ? "Pause" : "Play");

function handlePlayPause() {
	// v1: no resume from pause — pause is a hard stop. Future iteration may
	// re-enter playback at the next sentence boundary.
	if (store.isPlaying) {
		store.stop();
	}
}

function handleClose() {
	store.stop();
}
</script>

<div data-mini-player-root style="display: contents;">
{#if store.isActive && store.track}
	<aside
		class="mini-player"
		role="region"
		aria-label="Audio playback"
		data-testid="tts-mini-player"
	>
		<div class="mini-player__icon" aria-hidden="true">
			<Headphones class="h-5 w-5" />
		</div>
		<div class="mini-player__body">
			<span class="mini-player__title">{store.track.title}</span>
			<span class="mini-player__source">
				{store.track.source === "summary" ? "Summary" : "Article"}
				{#if store.isLoading} · loading…{/if}
			</span>
		</div>
		<button
			type="button"
			class="mini-player__btn"
			aria-label={playPauseLabel}
			onclick={handlePlayPause}
			disabled={store.isLoading}
		>
			{#if store.isLoading}
				<Loader2 class="h-4 w-4 animate-spin" aria-hidden="true" />
			{:else if store.isPlaying}
				<Pause class="h-4 w-4" aria-hidden="true" />
			{:else}
				<Play class="h-4 w-4" aria-hidden="true" />
			{/if}
		</button>
		<button
			type="button"
			class="mini-player__btn"
			aria-label="Close"
			onclick={handleClose}
		>
			<X class="h-4 w-4" aria-hidden="true" />
		</button>
	</aside>
{/if}
</div>

<style>
	.mini-player {
		position: fixed;
		left: 0;
		right: 0;
		/* Sit above MobileBottomNav (z-50, ~44px tall + safe area). */
		bottom: calc(2.75rem + env(safe-area-inset-bottom, 0px));
		z-index: 40;
		display: flex;
		align-items: center;
		gap: 0.6rem;
		min-height: 3.5rem;
		padding: 0.4rem 0.75rem;
		background: var(--surface-2, #f5f4f1);
		border-top: 1px solid var(--surface-border, #c8c8c8);
		box-shadow: 0 -4px 12px rgba(0, 0, 0, 0.04);
	}

	@media (min-width: 768px) {
		/* Desktop has no bottom-nav; sit flush against the viewport bottom. */
		.mini-player {
			bottom: 0;
		}
	}

	.mini-player__icon {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 2.25rem;
		height: 2.25rem;
		background: var(--surface-bg, #faf9f7);
		border: 1px solid var(--surface-border, #c8c8c8);
		color: var(--alt-primary, #2f4f4f);
		flex-shrink: 0;
	}

	.mini-player__body {
		display: flex;
		flex-direction: column;
		min-width: 0;
		flex: 1;
	}

	.mini-player__title {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--alt-charcoal, #1a1a1a);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.mini-player__source {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash, #999);
		letter-spacing: 0.04em;
	}

	.mini-player__btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 2.5rem;
		height: 2.5rem;
		background: var(--surface-bg, #faf9f7);
		border: 1px solid var(--surface-border, #c8c8c8);
		color: var(--alt-charcoal, #1a1a1a);
		cursor: pointer;
		transition: background 0.15s ease;
		flex-shrink: 0;
	}
	.mini-player__btn:hover:not(:disabled),
	.mini-player__btn:focus-visible {
		background: var(--surface-hover, #f3f1ed);
		outline: none;
	}
	.mini-player__btn:disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}
</style>

<script lang="ts">
import * as Sheet from "$lib/components/ui/sheet";
import type { TtsSource } from "$lib/stores/ttsPlayback.svelte";

interface Props {
	open: boolean;
	hasSummary: boolean;
	hasBody: boolean;
	speed: number;
	speedChoices: readonly number[];
	onClose: () => void;
	onStart: (source: TtsSource, speed: number) => void;
}

let {
	open,
	hasSummary,
	hasBody,
	speed,
	speedChoices,
	onClose,
	onStart,
}: Props = $props();

// Source / speed are bound to props through effects so opening the sheet
// always reflects the latest availability + persisted preferences.
let source = $state<TtsSource>("body");
let selectedSpeed = $state<number>(1.0);

$effect(() => {
	selectedSpeed = speed;
});

$effect(() => {
	if (!hasBody && hasSummary) {
		source = "summary";
	} else if (hasBody && !hasSummary) {
		source = "body";
	}
});

const canStart = $derived(
	(source === "summary" && hasSummary) || (source === "body" && hasBody),
);

function handleStart() {
	if (!canStart) return;
	onStart(source, selectedSpeed);
}
</script>

<Sheet.Root bind:open onOpenChange={(v) => !v && onClose()}>
	<Sheet.Content
		side="bottom"
		class="max-h-[75dvh] border-t border-[var(--surface-border,#c8c8c8)] w-full max-w-full sm:max-w-full p-0 gap-0 flex flex-col overflow-hidden"
		style="background: var(--surface-bg, #faf9f7) !important; border-radius: 0;"
		data-testid="tts-setup-sheet"
	>
		<Sheet.Header class="border-b border-[var(--surface-border,#c8c8c8)] px-4 py-3">
			<Sheet.Title
				class="font-[var(--font-body)] text-[0.65rem] font-bold uppercase tracking-[0.12em] text-[var(--alt-ash,#999)] m-0"
			>
				Listen to article
			</Sheet.Title>
		</Sheet.Header>

		<div class="px-4 py-4 flex flex-col gap-5">
			<fieldset class="border-0 p-0 m-0 flex flex-col gap-2">
				<legend class="font-[var(--font-mono)] text-[0.65rem] font-semibold uppercase tracking-[0.14em] text-[var(--alt-ash,#999)] mb-1.5">
					Source
				</legend>
				<div class="grid grid-cols-2 gap-2" role="radiogroup" aria-label="Source">
					<button
						type="button"
						role="radio"
						aria-checked={source === "summary"}
						aria-label="Summary"
						disabled={!hasSummary}
						onclick={() => { if (hasSummary) source = "summary"; }}
						class="source-choice"
						class:source-choice--active={source === "summary"}
						class:source-choice--disabled={!hasSummary}
					>
						<span class="source-choice__label">Summary</span>
						<span class="source-choice__hint">
							{hasSummary ? "AI-generated overview" : "Not generated yet"}
						</span>
					</button>
					<button
						type="button"
						role="radio"
						aria-checked={source === "body"}
						aria-label="Article"
						disabled={!hasBody}
						onclick={() => { if (hasBody) source = "body"; }}
						class="source-choice"
						class:source-choice--active={source === "body"}
						class:source-choice--disabled={!hasBody}
					>
						<span class="source-choice__label">Article</span>
						<span class="source-choice__hint">
							{hasBody ? "Full body text" : "Fetch the article first"}
						</span>
					</button>
				</div>
			</fieldset>

			<fieldset class="border-0 p-0 m-0 flex flex-col gap-2">
				<legend class="font-[var(--font-mono)] text-[0.65rem] font-semibold uppercase tracking-[0.14em] text-[var(--alt-ash,#999)] mb-1.5">
					Speed
				</legend>
				<div class="flex flex-wrap gap-2" role="radiogroup" aria-label="Playback speed">
					{#each speedChoices as choice}
						<button
							type="button"
							role="radio"
							aria-checked={selectedSpeed === choice}
							aria-label="{choice}x"
							onclick={() => (selectedSpeed = choice)}
							class="speed-chip"
							class:speed-chip--active={selectedSpeed === choice}
						>
							{choice}x
						</button>
					{/each}
				</div>
			</fieldset>

			<p class="font-[var(--font-body)] text-[0.7rem] text-[var(--alt-slate,#666)] italic m-0">
				Playback may pause when the tab is in the background. Turn off VoiceOver to avoid double narration.
			</p>
		</div>

		<div class="px-4 pb-[calc(1rem+env(safe-area-inset-bottom,0px))] pt-2 flex justify-end">
			<button
				type="button"
				class="action-btn action-btn--primary"
				disabled={!canStart}
				onclick={handleStart}
			>
				Start listening
			</button>
		</div>

	</Sheet.Content>
</Sheet.Root>

<style>
	.source-choice {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 0.25rem;
		padding: 0.75rem 0.9rem;
		min-height: 4rem;
		background: var(--surface-bg, #faf9f7);
		border: 1px solid var(--surface-border, #c8c8c8);
		text-align: left;
		cursor: pointer;
		transition: background 0.15s ease, border-color 0.15s ease;
	}
	.source-choice:hover:not(.source-choice--disabled) {
		background: var(--surface-hover, #f3f1ed);
	}
	.source-choice--active {
		border-color: var(--alt-primary, #2f4f4f);
		border-left-width: 3px;
		padding-left: calc(0.9rem - 2px);
	}
	.source-choice--disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}
	.source-choice__label {
		font-family: var(--font-body);
		font-size: 0.9rem;
		font-weight: 600;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.source-choice__hint {
		font-family: var(--font-body);
		font-size: 0.7rem;
		color: var(--alt-ash, #999);
	}

	.speed-chip {
		min-width: 3rem;
		min-height: 2.25rem;
		padding: 0.25rem 0.6rem;
		font-family: var(--font-mono);
		font-size: 0.8rem;
		background: var(--surface-bg, #faf9f7);
		border: 1px solid var(--surface-border, #c8c8c8);
		color: var(--alt-charcoal, #1a1a1a);
		cursor: pointer;
		transition: background 0.15s ease;
	}
	.speed-chip:hover {
		background: var(--surface-hover, #f3f1ed);
	}
	.speed-chip--active {
		background: var(--alt-primary, #2f4f4f);
		color: var(--surface-bg, #faf9f7);
		border-color: var(--alt-primary, #2f4f4f);
	}

	.action-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 2.5rem;
		padding: 0.4rem 1rem;
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		cursor: pointer;
		transition: background 0.15s ease, color 0.15s ease;
	}
	.action-btn--primary {
		background: var(--alt-primary, #2f4f4f);
		border: 1px solid var(--alt-primary, #2f4f4f);
		color: var(--surface-bg, #faf9f7);
	}
	.action-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
</style>

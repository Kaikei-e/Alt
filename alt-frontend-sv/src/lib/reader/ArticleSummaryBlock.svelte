<script lang="ts">
import { Sparkles } from "@lucide/svelte";
import type { ArticleReader } from "./createArticleReader.svelte.js";

interface Props {
	reader: ArticleReader;
}

const { reader }: Props = $props();
</script>

{#if reader.summarizer.summary}
	<section class="ai-summary" data-testid="ai-summary" aria-label="AI summary">
		<p class="ai-summary__kicker">
			<Sparkles class="h-[0.75rem] w-[0.75rem]" />
			<span>AI SUMMARY</span>
		</p>
		<div class="ai-summary__body">{reader.summarizer.summary}</div>
		{#if reader.canPlaySpeech}
			<div class="summary-speech-controls">
				<button
					type="button"
					class="speech-btn"
					data-testid="summary-speech-button"
					disabled={reader.speech.state === "unavailable"}
					onclick={reader.toggleSpeech}
				>
					{#if reader.speech.state === "playing" || reader.speech.state === "loading"}
						Stop
					{:else}
						Play summary
					{/if}
				</button>
				<span
					class="speech-status"
					data-testid="summary-speech-status"
					role="status"
					aria-live="polite"
				>{reader.speechStatusText}</span>
			</div>
		{/if}
		{#if reader.summarizer.summaryError}
			<p
				class="ai-summary__interrupted"
				data-testid="summary-interrupted"
				role="alert"
			>
				Stream interrupted. Summary may be incomplete.
			</p>
		{/if}
	</section>
{:else if reader.summarizer.summaryError}
	<section class="ai-summary ai-summary--error" role="alert">
		<p class="ai-summary__kicker">SUMMARIZE ERROR</p>
		<div class="ai-summary__body">{reader.summarizer.summaryError}</div>
	</section>
{/if}

<style>
	.ai-summary {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		padding: 1rem 1.1rem 1.1rem;
		background: var(--surface-2, #f5f4f1);
		border: 1px solid var(--surface-border, #c8c8c8);
		border-left: 3px solid var(--alt-primary, #2f4f4f);
	}

	.ai-summary--error {
		border-left-color: var(--alt-error, #8c1d1d);
	}

	.ai-summary__kicker {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		margin: 0;
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-ash, #999999);
	}

	.ai-summary__interrupted {
		margin: 0;
		padding-left: 0.75rem;
		border-left: 3px solid var(--alt-error, #8c1d1d);
		font-family: var(--font-body);
		font-size: 0.82rem;
		color: var(--alt-error, #8c1d1d);
	}

	.ai-summary__body {
		font-family: var(--font-body);
		font-size: 1rem;
		line-height: 1.75;
		color: var(--alt-charcoal, #1a1a1a);
		white-space: pre-wrap;
		word-break: break-word;
		overflow-wrap: anywhere;
	}

	.summary-speech-controls {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.speech-btn {
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-primary, #2f4f4f);
		background: transparent;
		border: 1px solid var(--surface-border, #c8c8c8);
		padding: 0.35rem 0.75rem;
		min-height: 36px;
		cursor: pointer;
		transition: background 0.15s ease;
	}

	.speech-btn:hover:not(:disabled) {
		background: var(--surface-hover, #f3f1ed);
	}

	.speech-btn:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	.speech-status {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-slate, #666666);
	}
</style>

<script lang="ts">
/**
 * MacroByline — second byline row on /loop that surfaces the day-to-week
 * (macro) layer of the user's cognitive state. ADR-000909 §Δ2: micro is the
 * intra-session OODA stage, meso is the surface bucket, macro is "what's
 * still in flight, what's accumulated for re-eval, what graduated this week".
 *
 * Inputs default to 0 / null when the session_state row hasn't yet been
 * recomputed by `macro_state_builder.go`. The line is omitted entirely if
 * all three counts are zero — empty macro state is a normal end-state per
 * canonical contract §14 and would otherwise read as visual noise.
 */

interface Props {
	activeContinueThreads?: number;
	pendingReviewCount?: number;
	recentInternalizedCount?: number;
	cognitiveLoadHint?: "light" | "medium" | "heavy";
}

const {
	activeContinueThreads = 0,
	pendingReviewCount = 0,
	recentInternalizedCount = 0,
	cognitiveLoadHint,
}: Props = $props();

const hasAny = $derived(
	activeContinueThreads > 0 ||
		pendingReviewCount > 0 ||
		recentInternalizedCount > 0,
);
</script>

{#if hasAny}
	<p
		class="macro-byline"
		aria-live="polite"
		data-testid="loop-macro-byline"
		data-cognitive-load-hint={cognitiveLoadHint ?? "unspecified"}
	>
		{#if activeContinueThreads > 0}
			<span class="macro-cell">
				<span class="macro-val">{activeContinueThreads}</span>
				<span class="macro-key">continuing</span>
			</span>
		{/if}
		{#if activeContinueThreads > 0 && pendingReviewCount > 0}
			<span class="macro-sep" aria-hidden="true">—</span>
		{/if}
		{#if pendingReviewCount > 0}
			<span class="macro-cell">
				<span class="macro-val">{pendingReviewCount}</span>
				<span class="macro-key">to review</span>
			</span>
		{/if}
		{#if (activeContinueThreads > 0 || pendingReviewCount > 0) && recentInternalizedCount > 0}
			<span class="macro-sep" aria-hidden="true">—</span>
		{/if}
		{#if recentInternalizedCount > 0}
			<span class="macro-cell">
				<span class="macro-val">{recentInternalizedCount}</span>
				<span class="macro-key">internalized this week</span>
			</span>
		{/if}
	</p>
{/if}

<style>
.macro-byline {
	margin: 0.2rem 0 0;
	font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
	font-size: 0.7rem;
	letter-spacing: 0.04em;
	color: var(--alt-slate, #666);
}
.macro-cell {
	display: inline-flex;
	align-items: baseline;
	gap: 0.25rem;
}
.macro-val {
	color: var(--alt-charcoal, #1a1a1a);
	font-weight: 600;
}
.macro-key {
	font-size: 0.66rem;
	text-transform: lowercase;
	letter-spacing: 0.06em;
	color: var(--alt-slate, #666);
}
.macro-sep {
	margin: 0 0.45rem;
	color: var(--alt-tertiary, #808080);
}
</style>

<script lang="ts">
import {
	Activity,
	AlertCircle,
	CalendarRange,
	Clock,
	Newspaper,
} from "@lucide/svelte";
import type {
	ServiceQuality,
	TodayDigestData,
} from "#lib/connect/knowledge_home.js";

interface Props {
	digest: TodayDigestData | null;
	serviceQuality?: ServiceQuality;
}

const { digest, serviceQuality = "full" }: Props = $props();

const isFallback = $derived(
	serviceQuality === "fallback" && digest?.digestFreshness === "unknown",
);
const distinctTopTags = $derived.by(() => {
	if (!digest?.topTags) return [];
	const themeLower = digest.primaryTheme?.trim().toLowerCase();
	return digest.topTags.filter(
		(t) => !themeLower || t.trim().toLowerCase() !== themeLower,
	);
});
</script>

{#if !digest}
	<div class="bar bar--muted">
		<AlertCircle class="h-4 w-4" style="color: var(--alt-ash);" />
		<span class="bar-message">Today's digest is still being prepared.</span>
	</div>
{:else if isFallback}
	<div class="bar bar--muted">
		<AlertCircle class="h-4 w-4" style="color: var(--alt-warning);" />
		<span class="bar-message">Digest section is temporarily unavailable or stale.</span>
	</div>
{:else}
	<div class="bar">
		<!-- Row 1: Action Shortcuts -->
		<div class="bar-actions">
			<a href="/recap/morning-letter" class="action-link">
				<Newspaper class="h-4 w-4" />
				Morning Letter
			</a>

			{#if digest.eveningPulseAvailable}
				<a href="/recap/evening-pulse" class="action-link">
					<Activity class="h-4 w-4" />
					Pulse
					{#if digest.needToKnowCount > 0}
						<span class="pulse-count" title="{digest.needToKnowCount} need-to-know">
							{digest.needToKnowCount}
						</span>
					{/if}
				</a>
			{:else}
				<span class="action-link action-link--disabled" title="No pulse content available today">
					<Activity class="h-4 w-4" />
					Pulse
				</span>
			{/if}

			{#if digest.weeklyRecapAvailable}
				<a href="/recap" class="action-link">
					<CalendarRange class="h-4 w-4" />
					Recap
				</a>
			{:else}
				<span class="action-link action-link--disabled" title="No recap available today">
					<CalendarRange class="h-4 w-4" />
					Recap
				</span>
			{/if}

			{#if digest.digestFreshness === "stale"}
				<span class="stale-indicator" title="Data may be outdated">
					<Clock class="h-3 w-3" />
					STALE
				</span>
			{/if}
		</div>

		<!-- Row 2: Figures + Tags -->
		<div class="bar-figures">
			<div class="figures-row">
				<span class="figure">
					<span class="figure-value">{digest.newArticles}</span>
					<span class="figure-label">NEW</span>
				</span>
				<span class="figure">
					<span class="figure-value">{digest.summarizedArticles}</span>
					<span class="figure-label">SUMMARIZED</span>
				</span>
				{#if digest.unsummarizedArticles > 0}
					<span class="figure">
							<span class="figure-value">{digest.unsummarizedArticles}</span>
						<span class="figure-label">PENDING</span>
					</span>
				{/if}
			</div>

			{#if digest.primaryTheme}
				<p class="bar-theme">
					{digest.primaryTheme}
				</p>
			{/if}

			{#if distinctTopTags.length > 0}
				<div class="flex items-center gap-1 ml-auto">
					{#each distinctTopTags.slice(0, 5) as tag}
						<span class="bar-tag">{tag}</span>
					{/each}
				</div>
			{/if}
		</div>
	</div>
{/if}


<style>
	.bar {
		display: flex;
		flex-direction: column;
		border-bottom: 1px solid var(--surface-border);
	}

	.bar--muted {
		flex-direction: row;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 0;
	}

	.bar-message {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-slate);
	}

	/* ── Row 1: Action shortcuts ── */
	.bar-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem;
		padding: 1rem 0 0.75rem;
	}

	.action-link {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		border: 1px solid var(--chip-border);
		background: var(--action-surface);
		padding: 0.45rem 0.85rem;
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-weight: 500;
		color: var(--interactive-text);
		text-decoration: none;
		transition: background 0.15s, color 0.15s;
	}

	.action-link:hover {
		background: var(--action-surface-hover);
		color: var(--interactive-text-hover);
	}

	.action-link--disabled {
		background: transparent;
		border-color: var(--surface-border);
		color: var(--alt-ash);
		cursor: default;
		pointer-events: none;
	}

	.pulse-count {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 1.15rem;
		height: 1.15rem;
		padding: 0 0.3rem;
		font-family: var(--font-mono);
		font-size: 0.66rem;
		font-weight: 600;
		background: var(--accent-emphasis-bg);
		color: var(--accent-emphasis-text);
		border: 1px solid var(--accent-emphasis-border);
	}

	.stale-indicator {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		margin-left: auto;
		font-family: var(--font-mono);
		font-size: 0.64rem;
		font-weight: 600;
		letter-spacing: 0.12em;
		color: var(--alt-warning);
	}

	/* ── Row 2: Figures strip ── */
	.bar-figures {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem 1rem;
		padding: 0 0 0.85rem;
	}

	.figures-row {
		display: flex;
		align-items: baseline;
		flex-wrap: wrap;
	}

	.figure {
		display: inline-flex;
		align-items: baseline;
		gap: 0.4rem;
		padding: 0 1rem;
		border-left: 1px solid var(--surface-border);
	}

	.figure:first-child {
		padding-left: 0;
		border-left: none;
	}

	.figure-value {
		font-family: var(--font-mono);
		font-size: 1rem;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		color: var(--alt-charcoal);
	}

	.figure-label {
		font-family: var(--font-mono);
		font-size: 0.64rem;
		font-weight: 600;
		letter-spacing: 0.12em;
		color: var(--alt-ash);
	}

	.bar-theme {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-slate);
	}

	.bar-tag {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--chip-text);
		border: 1px solid var(--chip-border);
		background: var(--chip-bg);
		padding: 0.08rem 0.5rem;
	}
</style>

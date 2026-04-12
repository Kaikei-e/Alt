<script lang="ts">
import {
	Activity,
	AlertCircle,
	CalendarRange,
	Clock,
	FileText,
	Newspaper,
	Sparkles,
} from "@lucide/svelte";
import type {
	ServiceQuality,
	TodayDigestData,
} from "$lib/connect/knowledge_home";

interface Props {
	digest: TodayDigestData | null;
	serviceQuality?: ServiceQuality;
}

const { digest, serviceQuality = "full" }: Props = $props();

const isFallback = $derived(
	serviceQuality === "fallback" && digest?.digestFreshness === "unknown",
);
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
					<Sparkles class="h-3.5 w-3.5 figure-icon" />
					<span class="figure-value">{digest.newArticles}</span>
					<span class="figure-label">NEW</span>
				</span>
				<span class="figure">
					<FileText class="h-3.5 w-3.5 figure-icon" />
					<span class="figure-value">{digest.summarizedArticles}</span>
					<span class="figure-label">SUMMARIZED</span>
				</span>
				{#if digest.unsummarizedArticles > 0}
					<span class="figure">
						<AlertCircle class="h-3.5 w-3.5 figure-icon" />
						<span class="figure-value">{digest.unsummarizedArticles}</span>
						<span class="figure-label">PENDING</span>
					</span>
				{/if}
			</div>

			{#if digest.primaryTheme || digest.topTags.length > 0}
				<p class="bar-theme">
					{digest.primaryTheme ?? digest.topTags.slice(0, 3).join(", ")}
				</p>
			{/if}

			{#if digest.topTags.length > 0}
				<div class="flex items-center gap-1 ml-auto">
					{#each digest.topTags.slice(0, 5) as tag}
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
		background: var(--surface-bg);
	}

	.bar--muted {
		flex-direction: row;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
	}

	.bar-message {
		font-family: var(--font-body);
		font-size: 0.875rem;
		color: var(--alt-slate);
	}

	/* ── Row 1: Action shortcuts ── */
	.bar-actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 1rem;
		border-bottom: 1px solid color-mix(in srgb, var(--surface-border) 30%, transparent);
	}

	.action-link {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		border: 1px solid var(--chip-border);
		background: var(--action-surface);
		padding: 0.5rem 0.75rem;
		font-family: var(--font-body);
		font-size: 0.875rem;
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
		border-color: var(--surface-border);
		color: color-mix(in srgb, var(--alt-slate) 55%, transparent);
		cursor: default;
		pointer-events: none;
	}

	.pulse-count {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 1.25rem;
		height: 1.25rem;
		padding: 0 0.25rem;
		font-size: 0.75rem;
		font-weight: 600;
		background: var(--accent-emphasis-bg);
		color: var(--accent-emphasis-text);
		border: 1px solid var(--accent-emphasis-border);
	}

	.stale-indicator {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		margin-left: auto;
		font-family: var(--font-mono);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		color: var(--alt-warning);
	}

	:global(.figure-icon) {
		color: var(--alt-ash);
	}

	/* ── Row 2: Figures bar ── */
	.bar-figures {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
		padding: 0.5rem 1rem;
	}

	.figures-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.figure {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
	}

	.figure-value {
		font-family: var(--font-mono);
		font-size: 1.1rem;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		color: var(--alt-charcoal);
	}

	.figure-label {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		color: var(--alt-ash);
	}

	.bar-theme {
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--alt-slate);
	}

	.bar-tag {
		border: 1px solid var(--chip-border);
		background: var(--chip-bg);
		padding: 0.125rem 0.5rem;
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--chip-text);
	}
</style>

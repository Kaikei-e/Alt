<script lang="ts">
type Props = {
	throughLine: string;
	targetDate: string;
	editionTimezone: string;
	isDegraded: boolean;
};

let { throughLine, targetDate, editionTimezone, isDegraded }: Props = $props();

const degradedLabel = "Partial edition";
</script>

<section class="through-line" data-role="through-line">
	<div class="through-line-meta">
		<span class="through-line-date">{targetDate}</span>
		<span class="through-line-divider" aria-hidden="true">·</span>
		<span class="through-line-tz">{editionTimezone}</span>
		{#if isDegraded}
			<span class="through-line-divider" aria-hidden="true">·</span>
			<span class="through-line-degraded" data-role="degraded-badge">{degradedLabel}</span>
		{/if}
	</div>
	<p class="through-line-text">{throughLine}</p>
	<div class="through-line-rule" aria-hidden="true"></div>
</section>

<style>
	.through-line {
		padding: 1.75rem 0 1rem;
		border-top: 3px double var(--alt-ink, #1a1a1a);
		margin-bottom: 1.25rem;
	}

	.through-line-meta {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.4rem;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.68rem;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
		margin-bottom: 0.65rem;
	}

	.through-line-date {
		color: var(--alt-ink, #1a1a1a);
		font-weight: 600;
	}

	.through-line-divider {
		color: var(--alt-ash, #c8c8c8);
	}

	.through-line-degraded {
		color: var(--alt-vermillion, #a83232);
		font-weight: 600;
	}

	.through-line-text {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: clamp(1.35rem, 2.4vw, 1.95rem);
		line-height: 1.3;
		letter-spacing: -0.01em;
		color: var(--alt-ink, #1a1a1a);
		margin: 0 0 0.85rem;
		font-weight: 700;
	}

	.through-line-rule {
		width: 40%;
		height: 1px;
		background: var(--surface-border, #c8c8c8);
	}
</style>

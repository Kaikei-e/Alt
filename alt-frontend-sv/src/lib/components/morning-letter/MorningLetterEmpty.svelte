<script lang="ts">
type Props = {
	requestedDate?: string | null;
	lastPublishedDate?: string | null;
	onViewLastPublished?: () => void;
	onRegenerate?: () => void;
	regenerating?: boolean;
	regenerateDisabledReason?: string | null;
};

let {
	requestedDate,
	lastPublishedDate = null,
	onViewLastPublished,
	onRegenerate,
	regenerating = false,
	regenerateDisabledReason = null,
}: Props = $props();

const hasLastPublished = $derived(Boolean(lastPublishedDate && !requestedDate));
const canRegenerate = $derived(Boolean(onRegenerate) && !regenerating);
</script>

<div class="empty-state" data-role="empty-state">
	<div class="empty-rule"></div>
	<span class="empty-label">Morning Letter</span>
	<h2 class="empty-heading">
		{#if requestedDate}No Edition For {requestedDate}{:else}Letter Not Yet Published{/if}
	</h2>
	<p class="empty-text">
		{#if requestedDate}
			We didn't compose an edition for {requestedDate}. Your through-line is still woven from the days that <em>did</em> ship — pick another date, or ask for a fresh run below.
		{:else}
			Your editorial projector hasn't ticked for today yet. You can read the most recent letter or ask the projector to run now.
		{/if}
	</p>

	{#if hasLastPublished}
		<button class="empty-ghost" type="button" onclick={onViewLastPublished}>
			Read the {lastPublishedDate} edition →
		</button>
	{/if}

	{#if onRegenerate}
		<button
			class="empty-cta"
			type="button"
			disabled={!canRegenerate}
			data-role="regenerate-cta"
			onclick={onRegenerate}
		>
			{#if regenerating}Composing…{:else}Generate today's letter{/if}
		</button>
		{#if regenerateDisabledReason}
			<p class="empty-disabled-reason">{regenerateDisabledReason}</p>
		{/if}
	{/if}

	<div class="empty-rule"></div>
</div>

<style>
	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 4rem 1rem;
		min-height: 40dvh;
		text-align: center;
		gap: 0.55rem;
	}

	.empty-rule {
		width: 120px;
		height: 1px;
		background: var(--surface-border, #c8c8c8);
	}

	.empty-label {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--alt-ash, #999);
		margin-top: 1rem;
	}

	.empty-heading {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.35rem;
		font-weight: 700;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0.3rem 0 0.2rem;
	}

	.empty-text {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.9rem;
		line-height: 1.55;
		color: var(--alt-slate, #666);
		max-width: 28rem;
		margin: 0 0 0.75rem;
	}

	.empty-ghost {
		background: transparent;
		border: none;
		color: var(--alt-ink, #1a1a1a);
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.82rem;
		letter-spacing: 0.03em;
		text-decoration: underline;
		text-underline-offset: 4px;
		cursor: pointer;
		padding: 0.25rem 0.4rem;
	}

	.empty-ghost:hover {
		color: var(--alt-vermillion, #a83232);
	}

	.empty-cta {
		margin-top: 0.4rem;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.78rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		padding: 0.65rem 1.35rem;
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--alt-paper, #f4f1ea);
		border: none;
		cursor: pointer;
	}

	.empty-cta:disabled {
		opacity: 0.55;
		cursor: not-allowed;
	}

	.empty-disabled-reason {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.72rem;
		color: var(--alt-ash, #999);
		margin: 0.2rem 0 0;
	}
</style>

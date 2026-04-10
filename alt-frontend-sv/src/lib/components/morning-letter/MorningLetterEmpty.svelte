<script lang="ts">
type Props = {
	requestedDate?: string | null;
};

let { requestedDate }: Props = $props();
</script>

<div class="empty-state" data-role="empty-state">
	<div class="empty-rule"></div>
	<span class="empty-label">Morning Letter</span>
	<h2 class="empty-heading">No Edition Available</h2>
	<p class="empty-text">
		{#if requestedDate}
			No letter was generated for {requestedDate}. Try a different date or check back later.
		{:else}
			The morning briefing hasn't been generated yet. Check back soon.
		{/if}
	</p>
	<div class="empty-rule"></div>
</div>

<style>
	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 4rem 1rem;
		min-height: 40vh;
		text-align: center;
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
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0.3rem 0 0.5rem;
	}

	.empty-text {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.85rem;
		line-height: 1.5;
		color: var(--alt-slate, #666);
		max-width: 24rem;
		margin: 0 0 1rem;
	}
</style>

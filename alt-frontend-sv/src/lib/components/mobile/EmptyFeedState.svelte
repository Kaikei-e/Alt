<script lang="ts">
</script>

<div
	role="region"
	aria-label="Empty feed state"
	class="empty-state"
>
	<div class="empty-ornament" aria-hidden="true">&#9670;</div>
	<h2 class="empty-heading">No Feeds Yet</h2>
	<p class="empty-body">
		Start by adding your first RSS feed to begin reading articles
		from your favorite sources.
	</p>
	<a href="/settings/feeds" class="empty-action">
		Add Your First Feed
	</a>
</div>

<style>
	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		min-height: 70vh;
		padding: 1.5rem;
		text-align: center;
	}

	.empty-ornament {
		font-size: 1.5rem;
		color: var(--surface-border);
		margin-bottom: 1rem;
	}

	.empty-heading {
		font-family: var(--font-display);
		font-size: 1.4rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0 0 0.5rem;
	}

	.empty-body {
		font-family: var(--font-body);
		font-size: 0.9rem;
		line-height: 1.6;
		color: var(--alt-slate);
		max-width: 320px;
		margin: 0 0 1.5rem;
	}

	.empty-action {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 0.6rem 1.25rem;
		min-height: 44px;
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
		text-decoration: none;
		transition:
			background 0.15s,
			color 0.15s;
	}

	.empty-action:active {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}
</style>

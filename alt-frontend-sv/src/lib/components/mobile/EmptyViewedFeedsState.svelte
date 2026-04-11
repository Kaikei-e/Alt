<script lang="ts">
</script>

<div
	role="region"
	aria-label="Empty morgue state"
	class="empty-state"
	data-role="morgue-empty-state"
>
	<div class="empty-ornament" aria-hidden="true">&#9670;</div>
	<h2 class="empty-heading">Nothing Filed Yet</h2>
	<p class="empty-body">
		Articles you read will appear in the morgue.
	</p>
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
		margin: 0;
	}
</style>

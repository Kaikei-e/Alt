<script lang="ts">
	/**
	 * Empty-Now surface. Per ADR-000831 and canonical contract §14, an empty Now is a
	 * normal terminal state, not an error. The user has processed everything in scope;
	 * do not pad the foreground with filler items — visual hierarchy depends on it.
	 */
</script>

<section class="empty-now" data-testid="loop-empty-now" role="status" aria-live="polite">
	<p class="kicker">Nothing more for now</p>
	<h2>You&rsquo;re caught up.</h2>
	<ul class="suggestions">
		<li>Check <strong>Continue</strong> for items already in progress.</li>
		<li>Open <strong>Review</strong> for a short retrospect.</li>
		<li>Waiting for new observations&hellip;</li>
	</ul>
</section>

<style>
	.empty-now {
		max-width: 44ch;
		margin: 2rem auto;
		padding: var(--space-lg, 1.5rem);
		text-align: center;
	}
	.kicker {
		font-family: var(--font-meta, "IBM Plex Mono", monospace);
		text-transform: uppercase;
		letter-spacing: 0.12em;
		font-size: 0.75rem;
		color: var(--fg-muted, #555);
		margin: 0 0 0.25rem;
	}
	h2 {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.5rem;
		margin: 0 0 1rem;
	}
	.suggestions {
		list-style: none;
		padding: 0;
		margin: 0;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.95rem;
		line-height: 1.7;
	}
</style>

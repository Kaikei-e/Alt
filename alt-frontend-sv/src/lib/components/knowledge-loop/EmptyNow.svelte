<script lang="ts">
/**
 * Empty-state block when the foreground plane has no entries. ADR-000831
 * §14 treats an empty Now as a normal terminal state — not an error. We
 * honour that with editorial restraint: a kicker, an italic serif
 * statement, and a short prose list of next moves. No padding with
 * filler items; the Now remains deliberately empty.
 */
</script>

<section
	class="empty-now"
	data-testid="loop-empty-now"
	role="status"
	aria-live="polite"
>
	<span class="kicker">Nothing in the Now</span>
	<h2 class="title">You&rsquo;re caught up.</h2>
	<p class="prose">
		Check <em>Continue</em> for items already in motion, open
		<em>Review</em> for a short retrospect, or wait for the next observation to land.
	</p>
</section>

<style>
	.empty-now {
		max-width: 44ch;
		margin: 2.5rem auto;
		padding: 1.8rem 1.2rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
		text-align: center;
	}
	.kicker {
		display: inline-block;
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
		margin-bottom: 0.55rem;
	}
	.title {
		font-family: var(--font-display, "Playfair Display", Georgia, serif);
		font-size: clamp(1.5rem, 3.5vw, 1.9rem);
		font-weight: 700;
		font-style: italic;
		line-height: 1.2;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0 0 0.75rem;
	}
	.prose {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.95rem;
		line-height: 1.7;
		color: var(--alt-slate, #666);
		margin: 0 auto;
		max-width: 38ch;
	}
	em {
		font-style: italic;
		color: var(--alt-charcoal, #1a1a1a);
	}
</style>

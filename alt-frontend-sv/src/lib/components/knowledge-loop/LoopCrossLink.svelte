<script lang="ts">
/**
 * One-line newspaper-style banner that points users from the Phase-0 surfaces
 * (`/home` Knowledge Home, `/feeds` raw inbox) to the primary cognitive-state
 * engine at `/loop`. Newspaper Style: en-dash separator, IBM Plex Mono,
 * no shadow or depth.
 */
type Props = {
	/**
	 * Short tag printed before the en-dash, e.g. "From Home" / "From Feeds".
	 */
	source?: string;
};

const { source = "Browse" }: Props = $props();
</script>

<a
	href="/loop"
	class="loop-cross-link"
	data-testid="loop-cross-link"
	aria-label="Open Knowledge Loop, the primary cognitive state engine"
>
	<span class="cross-link__source">{source}</span>
	<span class="cross-link__sep" aria-hidden="true">—</span>
	<span class="cross-link__label">Open your Knowledge Loop</span>
	<span class="cross-link__arrow" aria-hidden="true">→</span>
</a>

<style>
.loop-cross-link {
	display: inline-flex;
	align-items: baseline;
	gap: 0.5rem;
	padding: 0.5rem 0.75rem;
	margin: 0.75rem 0;
	font-family: var(--font-mono, "IBM Plex Mono", monospace);
	font-size: 0.8125rem;
	letter-spacing: 0.04em;
	color: var(--alt-primary, #2f4f4f);
	background: transparent;
	border-top: 1px solid currentColor;
	border-bottom: 1px solid currentColor;
	text-decoration: none;
	transition: background-color 0.15s ease;
}
.loop-cross-link:hover,
.loop-cross-link:focus-visible {
	background: rgba(47, 79, 79, 0.04);
	text-decoration: underline;
	text-underline-offset: 0.25em;
}
.loop-cross-link:focus-visible {
	outline: 2px solid var(--alt-primary, #2f4f4f);
	outline-offset: 2px;
}
.cross-link__source {
	font-size: 0.6875rem;
	text-transform: uppercase;
	letter-spacing: 0.12em;
	color: var(--alt-secondary, #696969);
}
.cross-link__sep {
	color: var(--alt-tertiary, #808080);
}
.cross-link__label {
	font-weight: 500;
}
.cross-link__arrow {
	margin-left: 0.25rem;
	color: var(--alt-secondary, #696969);
}

@media (prefers-reduced-motion: reduce) {
	.loop-cross-link {
		transition: none;
	}
}
</style>

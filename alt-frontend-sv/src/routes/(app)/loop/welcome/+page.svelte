<script lang="ts">
import { enhance } from "$app/forms";

type Lens = "research" | "browse" | "decide" | "recall";

const lensOptions: { id: Lens; label: string; blurb: string }[] = [
	{ id: "research", label: "Research", blurb: "Continuing over new" },
	{ id: "browse", label: "Browse", blurb: "What's now" },
	{ id: "decide", label: "Decide", blurb: "Now + Continue + Changed" },
	{ id: "recall", label: "Recall", blurb: "Review-led" },
];

let selectedLens: Lens = $state("browse");
</script>

<section class="welcome" data-testid="loop-welcome">
	<header class="masthead">
		<p class="masthead__byline">Alt — Knowledge Loop</p>
		<h1 class="masthead__title">Knowledge Loop</h1>
		<p class="masthead__rule">— Volume I, A Cognitive State Engine —</p>
	</header>

	<div class="leader">
		<p>Your cognitive state, not your inbox.</p>
		<p>
			What's now, what continues, what changed, and what's worth a second look.
		</p>
		<p>Pick a lens — you can switch any time.</p>
	</div>

	<form method="POST" use:enhance class="lens-form">
		<fieldset class="lens-picker">
			<legend class="sr-only">Pick your reading lens</legend>
			{#each lensOptions as option (option.id)}
				<label
					class="lens-option"
					class:selected={selectedLens === option.id}
					data-lens={option.id}
				>
					<input
						type="radio"
						name="lens"
						value={option.id}
						checked={selectedLens === option.id}
						onchange={() => (selectedLens = option.id)}
					/>
					<span class="lens-option__label">{option.label}</span>
					<span class="lens-option__blurb">{option.blurb}</span>
				</label>
			{/each}
		</fieldset>

		<button type="submit" class="start-cta">Start →</button>
	</form>
</section>

<style>
.welcome {
	max-width: 38rem;
	margin: 0 auto;
	padding: 3rem 1.5rem 4rem;
	color: var(--alt-primary, #2f4f4f);
	font-family: var(--font-body, "Source Sans 3", sans-serif);
}

.masthead {
	border-bottom: 2px solid currentColor;
	padding-bottom: 1rem;
	margin-bottom: 2rem;
	text-align: center;
}
.masthead__byline {
	font-family: var(--font-mono, "IBM Plex Mono", monospace);
	font-size: 0.75rem;
	letter-spacing: 0.12em;
	text-transform: uppercase;
	color: var(--alt-secondary, #696969);
	margin: 0 0 0.25rem;
}
.masthead__title {
	font-family: var(--font-display, "Playfair Display", serif);
	font-size: clamp(2.25rem, 5vw, 3.5rem);
	font-weight: 600;
	margin: 0;
	line-height: 1.05;
	letter-spacing: -0.01em;
}
.masthead__rule {
	font-family: var(--font-mono, "IBM Plex Mono", monospace);
	font-size: 0.75rem;
	letter-spacing: 0.08em;
	color: var(--alt-secondary, #696969);
	margin: 0.5rem 0 0;
}

.leader {
	margin-bottom: 2.25rem;
}
.leader p {
	font-size: 1.0625rem;
	line-height: 1.55;
	margin: 0 0 0.5rem;
}
.leader p:first-child {
	font-family: var(--font-display, "Playfair Display", serif);
	font-size: 1.25rem;
	font-style: italic;
	margin-bottom: 1rem;
}

.lens-form {
	display: flex;
	flex-direction: column;
	gap: 1.5rem;
}

.lens-picker {
	border: none;
	padding: 0;
	margin: 0;
	display: grid;
	grid-template-columns: repeat(2, 1fr);
	gap: 0.75rem;
}

.lens-option {
	display: flex;
	flex-direction: column;
	gap: 0.25rem;
	padding: 0.85rem 1rem;
	border: 1px solid var(--alt-tertiary, #808080);
	cursor: pointer;
	background: transparent;
	font-family: var(--font-mono, "IBM Plex Mono", monospace);
	position: relative;
	transition:
		background-color 0.15s ease,
		border-color 0.15s ease;
}
.lens-option:hover {
	background: rgba(47, 79, 79, 0.04);
}
.lens-option.selected {
	background: rgba(47, 79, 79, 0.08);
	border-color: var(--alt-primary, #2f4f4f);
	border-width: 1.5px;
	padding: calc(0.85rem - 0.5px) calc(1rem - 0.5px);
}
.lens-option input {
	position: absolute;
	width: 1px;
	height: 1px;
	overflow: hidden;
	clip: rect(0 0 0 0);
	white-space: nowrap;
}
.lens-option__label {
	font-size: 0.875rem;
	font-weight: 600;
	letter-spacing: 0.04em;
	text-transform: uppercase;
}
.lens-option__blurb {
	font-size: 0.75rem;
	color: var(--alt-secondary, #696969);
}

.start-cta {
	align-self: flex-start;
	padding: 0.75rem 1.75rem;
	font-family: var(--font-mono, "IBM Plex Mono", monospace);
	font-size: 0.95rem;
	letter-spacing: 0.06em;
	background: var(--alt-primary, #2f4f4f);
	color: var(--alt-bg, #faf9f7);
	border: none;
	cursor: pointer;
	transition: background-color 0.15s ease;
}
.start-cta:hover {
	background: #1f3939;
}
.start-cta:focus-visible {
	outline: 2px solid var(--alt-primary, #2f4f4f);
	outline-offset: 3px;
}

.sr-only {
	position: absolute;
	width: 1px;
	height: 1px;
	padding: 0;
	margin: -1px;
	overflow: hidden;
	clip: rect(0 0 0 0);
	white-space: nowrap;
	border: 0;
}

@media (prefers-reduced-motion: reduce) {
	.lens-option,
	.start-cta {
		transition: none;
	}
}
</style>

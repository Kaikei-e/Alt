<script lang="ts">
	import LoopSurfacePlane from "$lib/components/knowledge-loop/LoopSurfacePlane.svelte";
	import LoopEntryTile from "$lib/components/knowledge-loop/LoopEntryTile.svelte";
	import EmptyNow from "$lib/components/knowledge-loop/EmptyNow.svelte";
	import type { PageData } from "./$types";

	let { data }: { data: PageData } = $props();

	const foreground = $derived(data.loop?.foregroundEntries ?? []);
	const sessionState = $derived(data.loop?.sessionState);
	const quality = $derived(data.loop?.overallServiceQuality ?? "unspecified");
</script>

<svelte:head>
	<title>Knowledge Loop</title>
</svelte:head>

<main class="loop-root" data-testid="knowledge-loop-root">
	<header class="loop-header">
		<p class="kicker">Knowledge Loop</p>
		<h1>Observe &middot; Orient &middot; Decide &middot; Act</h1>
		{#if sessionState}
			<p class="session-hint" aria-live="polite">
				Current stage: <strong>{sessionState.currentStage}</strong>
			</p>
		{/if}
	</header>

	{#if data.error}
		<p class="loop-error" role="status">Loop unavailable: {data.error}</p>
	{:else if foreground.length === 0}
		<EmptyNow />
	{:else}
		<LoopSurfacePlane plane="foreground">
			{#each foreground as entry (entry.entryKey)}
				<LoopEntryTile {entry} />
			{/each}
		</LoopSurfacePlane>
	{/if}

	{#if quality !== "full" && quality !== "unspecified"}
		<p class="quality-banner" role="status">Service quality: {quality}</p>
	{/if}
</main>

<style>
	.loop-root {
		max-width: 72ch;
		margin: 0 auto;
		padding: var(--space-lg, 1.5rem) var(--space-md, 1rem);
	}
	.loop-header {
		margin-bottom: var(--space-lg, 1.5rem);
	}
	.kicker {
		font-family: var(--font-meta, "IBM Plex Mono", monospace);
		text-transform: uppercase;
		letter-spacing: 0.12em;
		font-size: 0.75rem;
		margin: 0;
	}
	h1 {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: clamp(1.75rem, 4vw, 2.5rem);
		margin: 0.25rem 0 0.5rem;
	}
	.session-hint {
		font-family: var(--font-meta, "IBM Plex Mono", monospace);
		font-size: 0.875rem;
		color: var(--fg-muted, #555);
	}
	.loop-error,
	.quality-banner {
		margin-top: var(--space-md, 1rem);
		padding: 0.75rem 1rem;
		border: 1px solid var(--border-muted, #ddd);
		font-family: var(--font-meta, "IBM Plex Mono", monospace);
		font-size: 0.875rem;
	}
</style>

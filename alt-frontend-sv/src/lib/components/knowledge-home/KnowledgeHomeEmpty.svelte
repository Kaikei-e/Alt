<script lang="ts">
import { AlertCircle, Inbox, Loader, Sparkles } from "@lucide/svelte";

export type EmptyReason =
	| "no_data"
	| "ingest_pending"
	| "lens_strict"
	| "search_strict"
	| "degraded"
	| "hard_error";

interface Props {
	reason?: EmptyReason | null;
	activeLensName?: string | null;
	onClearLens?: () => void;
}

const { reason = null, activeLensName = null, onClearLens }: Props = $props();

const config = $derived.by(() => {
	if (reason === "hard_error") {
		return {
			icon: AlertCircle,
			title: "Unable to load Knowledge Home",
			description: "An error occurred while loading. Please try again later.",
			showClearLens: false,
		};
	}
	if (reason === "lens_strict" && activeLensName) {
		return {
			icon: Sparkles,
			title: `No matches in ${activeLensName}`,
			description:
				"This lens does not match any articles right now. Clear the lens or adjust its filters.",
			showClearLens: true,
		};
	}
	if (reason === "search_strict") {
		return {
			icon: Sparkles,
			title: "No matches for this search",
			description:
				"Try broadening the query or clearing search mode to return to the main stream.",
			showClearLens: false,
		};
	}
	if (reason === "degraded") {
		return {
			icon: AlertCircle,
			title: "Only partial candidates are available",
			description:
				"Some services are degraded right now, so the stream may be thinner than usual.",
			showClearLens: false,
		};
	}
	if (reason === "ingest_pending") {
		return {
			icon: Loader,
			title: "Articles are being processed",
			description:
				"New articles are flowing in and being summarized. They will appear here shortly.",
			showClearLens: false,
		};
	}
	if (reason === "no_data") {
		return {
			icon: Inbox,
			title: "No articles yet",
			description:
				"Subscribe to feeds to start receiving articles in your Knowledge Home.",
			showClearLens: false,
		};
	}
	return {
		icon: Sparkles,
		title: "Your knowledge is warming up",
		description:
			"New articles and insights will appear here as they arrive and get processed.",
		showClearLens: false,
	};
});
</script>

<div class="flex flex-col items-center justify-center py-16 text-center">
	<config.icon
		class="h-10 w-10 text-[var(--text-secondary)] mb-4 {reason === 'ingest_pending' ? 'animate-spin' : ''}"
	/>
	<h3 class="text-base font-medium text-[var(--text-primary)] mb-1">
		{config.title}
	</h3>
	<p class="text-sm text-[var(--text-secondary)] max-w-sm">
		{config.description}
	</p>
	{#if config.showClearLens && onClearLens}
		<button
			class="mt-4 inline-flex items-center rounded-full border border-[var(--surface-border)] px-4 py-2 text-sm text-[var(--text-primary)] transition-colors hover:border-[var(--accent-primary)] hover:text-[var(--accent-primary)]"
			onclick={onClearLens}
		>
			Clear lens
		</button>
	{/if}
</div>

<script lang="ts">
import { onMount } from "svelte";
import { page } from "$app/stores";
import { useViewport } from "$lib/stores/viewport.svelte";
import { resolveAugurEntry } from "$lib/utils/augur-entry";

// Desktop components
import AugurChat from "$lib/components/desktop/augur/AugurChat.svelte";

// Mobile components
import ChatWindow from "$lib/components/mobile/search/ChatWindow.svelte";

const { isDesktop } = useViewport();

const augurEntry = $derived(
	resolveAugurEntry({
		q: $page.url.searchParams.get("q"),
		context: $page.url.searchParams.get("context"),
		articleId: $page.url.searchParams.get("articleId"),
	}),
);

// iOS Safari: prevent elastic bounce by blocking touchmove outside scroll container
onMount(() => {
	if (isDesktop) return;

	document.documentElement.classList.add("augur-page");

	function isScrollable(el: HTMLElement): boolean {
		const style = window.getComputedStyle(el);
		const oy = style.overflowY;
		return (
			(oy === "auto" || oy === "scroll") && el.scrollHeight > el.clientHeight
		);
	}

	function onTouchMove(e: TouchEvent) {
		// Allow scroll inside any scrollable container (thread, sheet, drawer, etc.)
		let node = e.target as HTMLElement | null;
		while (node && node !== document.documentElement) {
			if (isScrollable(node)) return;
			node = node.parentElement;
		}
		e.preventDefault();
	}

	document.addEventListener("touchmove", onTouchMove, { passive: false });

	return () => {
		document.documentElement.classList.remove("augur-page");
		document.removeEventListener("touchmove", onTouchMove);
	};
});
</script>

<svelte:head>
	<title>Ask Augur - Alt</title>
</svelte:head>

{#if isDesktop}
	<AugurChat
		initialContext={augurEntry.initialDraft}
		initialQuestion={augurEntry.initialMessage}
		onConversationIdChange={(id) => {
			// Reflect the persisted id in the URL without remounting the
			// component so a reload resumes the same conversation.
			if (typeof history !== "undefined") {
				history.replaceState(history.state, "", `/augur/${id}`);
			}
		}}
	/>
{:else}
	<div class="augur-mobile-shell">
		<ChatWindow
			initialContext={augurEntry.initialDraft}
			initialQuestion={augurEntry.initialMessage}
		/>
	</div>
{/if}

<style>
	/* Prevent body overflow on iOS — no position:fixed, just overflow control */
	:global(html.augur-page),
	:global(html.augur-page body) {
		overflow: hidden !important;
	}

	.augur-mobile-shell {
		height: 100dvh;
		overflow: hidden;
	}
</style>

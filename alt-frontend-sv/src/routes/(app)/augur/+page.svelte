<script lang="ts">
import { replaceState } from "$app/navigation";
import { page } from "$app/stores";
import AugurChat from "$lib/components/desktop/augur/AugurChat.svelte";
import { isDesktop } from "$lib/stores/viewport.svelte";
import { resolveAugurEntry } from "$lib/utils/augur-entry";

const augurEntry = $derived(
	resolveAugurEntry({
		q: $page.url.searchParams.get("q"),
		context: $page.url.searchParams.get("context"),
		articleId: $page.url.searchParams.get("articleId"),
	}),
);

// `augur-page` pins `overflow: hidden` on <html> so the phone shell — which is
// `position: fixed` — does not scroll the document behind itself. That is a
// property of the viewport, not of the mount, so it has to follow the viewport:
// `onMount` added it once and only dropped it on unmount, which left the class
// stuck on after a rotation into landscape (a desktop layout that could not
// scroll) and never applied it when a desktop session was rotated onto a phone.
$effect(() => {
	if (isDesktop()) return;

	document.documentElement.classList.add("augur-page");

	return () => {
		document.documentElement.classList.remove("augur-page");
	};
});
</script>

<svelte:head>
	<title>Ask Augur - Alt</title>
</svelte:head>

<!--
	One chat, mounted once. It used to be `{#if isDesktop()}` over two separately
	written chats, so every rotation destroyed the thread and re-asked the
	question still sitting in `?q=`. The phone's full-bleed shell is a width, so
	it is a media query here rather than a second component.
-->
<div class="augur-frame">
	<AugurChat
		initialContext={augurEntry.initialDraft}
		initialQuestion={augurEntry.initialMessage}
		onConversationIdChange={(id) => {
			// Reflect the persisted id in the address bar without navigating, so a
			// reload resumes the same conversation and the streaming component is
			// left alone. `replaceState` from `$app/navigation` rather than
			// `history.replaceState`, which SvelteKit's router asks callers not to
			// touch (it owns the history state it stores there).
			replaceState(`/augur/${id}`, {});
		}}
	/>
</div>

<style>
	/* Prevent body overflow on iOS — no position:fixed, just overflow control */
	:global(html.augur-page),
	:global(html.augur-page body) {
		overflow: hidden !important;
	}

	.augur-frame {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		bottom: calc(2.75rem + env(safe-area-inset-bottom, 0px));
		overflow: hidden;
	}

	/* The same 48rem the `md:` utilities and `isDesktop()` use. */
	@media (min-width: 48rem) {
		.augur-frame {
			position: static;
			overflow: visible;
		}
	}
</style>

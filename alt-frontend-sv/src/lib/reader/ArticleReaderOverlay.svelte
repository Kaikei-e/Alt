<script lang="ts">
import { page } from "$app/state";
import ArticleReaderOverlaySession from "./ArticleReaderOverlaySession.svelte";

const overlayState = $derived(page.state.reader ?? null);
</script>

{#if overlayState}
	{#key overlayState.sessionId}
		<ArticleReaderOverlaySession {overlayState} />
	{/key}
{/if}

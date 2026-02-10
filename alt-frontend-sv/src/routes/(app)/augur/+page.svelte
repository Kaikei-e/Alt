<script lang="ts">
import { useViewport } from "$lib/stores/viewport.svelte";

// Desktop components
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import AugurChat from "$lib/components/desktop/augur/AugurChat.svelte";

// Mobile components
import ChatWindow from "$lib/components/mobile/search/ChatWindow.svelte";

const { isDesktop } = useViewport();
</script>

<svelte:head>
	<title>Ask Augur - Alt</title>
</svelte:head>

{#if isDesktop}
	<PageHeader title="Ask Augur" description="Query your knowledge base with AI" />
	<AugurChat />
{:else}
	<div class="h-[calc(100vh-64px)] md:h-[calc(100vh-80px)] w-full overflow-hidden">
		<ChatWindow />
	</div>
{/if}

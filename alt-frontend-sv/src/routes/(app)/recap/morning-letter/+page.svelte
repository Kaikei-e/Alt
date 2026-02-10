<script lang="ts">
import { useViewport } from "$lib/stores/viewport.svelte";

// Desktop components
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import DesktopMorningLetterChat from "$lib/components/desktop/morning-letter/MorningLetterChat.svelte";

// Mobile components
import MobileMorningLetterChat from "$lib/components/mobile/morning-letter/MorningLetterChat.svelte";

const { isDesktop } = useViewport();
</script>

<svelte:head>
	<title>Morning Letter - Alt</title>
</svelte:head>

{#if isDesktop}
	<PageHeader title="Morning Letter" description="Ask about today's news" />
	<DesktopMorningLetterChat withinHours={24} />
{:else}
	<div class="min-h-[100dvh] flex flex-col" style="background: var(--app-bg);">
		<div class="flex-1">
			<MobileMorningLetterChat withinHours={24} />
		</div>
	</div>
{/if}

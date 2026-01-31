<script lang="ts">
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import MorningLetterChat from "$lib/components/desktop/morning-letter/MorningLetterChat.svelte";
</script>

<svelte:head>
	<title>Morning Letter - Alt</title>
</svelte:head>

<PageHeader title="Morning Letter" description="Ask about today's news" />

<MorningLetterChat withinHours={24} />

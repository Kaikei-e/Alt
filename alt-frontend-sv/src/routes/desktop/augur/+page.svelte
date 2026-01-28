<script lang="ts">
	import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
	import AugurChat from "$lib/components/desktop/augur/AugurChat.svelte";
</script>

<svelte:head>
	<title>Ask Augur - Alt</title>
</svelte:head>

<PageHeader title="Ask Augur" description="Query your knowledge base with AI" />

<AugurChat />

<script lang="ts">
import { goto } from "$app/navigation";
import { onMount } from "svelte";
import ConversationList from "$lib/components/desktop/augur/ConversationList.svelte";
import { useAugurHistory } from "$lib/hooks/useAugurHistory.svelte";

const history = useAugurHistory({ pageSize: 20 });

onMount(() => {
	void history.refresh();
});

async function handleDelete(id: string) {
	await history.remove(id);
}

function handleOpen(id: string) {
	void goto(`/augur/${id}`);
}

function handleStartNew() {
	void goto("/augur");
}

async function handleLoadMore() {
	await history.loadMore();
}
</script>

<svelte:head>
	<title>Ask Augur · History</title>
</svelte:head>

<ConversationList
	conversations={history.conversations}
	isLoading={history.isLoading}
	errorMessage={history.errorMessage}
	hasMore={history.hasMore}
	onOpen={handleOpen}
	onDelete={handleDelete}
	onLoadMore={handleLoadMore}
	onStartNew={handleStartNew}
/>

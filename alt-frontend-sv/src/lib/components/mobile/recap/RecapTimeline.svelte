<script lang="ts">
	import type { RecapGenre } from "$lib/schema/recap";
	import RecapCard from "./RecapCard.svelte";

	interface Props {
		genres: RecapGenre[];
	}

	const { genres }: Props = $props();
</script>

<div class="flex flex-col gap-4" data-testid="recap-timeline">
	{#each genres as genre}
		<RecapCard {genre} />
	{/each}
</div>


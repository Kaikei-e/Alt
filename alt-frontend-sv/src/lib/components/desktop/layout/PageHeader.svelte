<script lang="ts">
let { title, description = "" }: { title: string; description?: string } =
	$props();
</script>

<header class="mb-6">
	<h1 class="text-2xl font-bold text-[var(--text-primary)]">{title}</h1>
	{#if description}
		<p class="text-sm text-[var(--text-secondary)] mt-1">{description}</p>
	{/if}
</header>

<script lang="ts">
	import type { Snippet } from "svelte";
	import DesktopLayout from "$lib/components/desktop/layout/DesktopLayout.svelte";

	let { children }: { children: Snippet } = $props();
</script>

<DesktopLayout>
	{@render children()}
</DesktopLayout>

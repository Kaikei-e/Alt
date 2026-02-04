<script lang="ts">
import type { Snippet } from "svelte";
import Sidebar from "./Sidebar.svelte";
import { cn } from "$lib/utils";

let { children, class: className = "" }: { children: Snippet; class?: string } =
	$props();
</script>

<div class="flex min-h-screen bg-[var(--surface-bg)]">
	<Sidebar />
	<main class={cn("flex-1 ml-60", className || "p-6")}>
		{@render children()}
	</main>
</div>

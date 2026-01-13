<script lang="ts">
import { onMount } from "svelte";
import "./layout.css";
import favicon from "$lib/assets/favicon.svg";
import { QueryClient, QueryClientProvider } from "@tanstack/svelte-query";
import { page } from "$app/state";
import { auth } from "$lib/stores/auth.svelte";

const { children } = $props();

// Sync auth store with page data (user from +layout.server.ts)
$effect(() => {
	const data = page.data;
	if (data && data.user !== undefined) {
		auth.setUser(data.user);
	}
});

// Signal hydration completion to hide splash screen
onMount(() => {
	document.body.classList.add("hydrated");
});

// Create QueryClient for TanStack Query
const queryClient = new QueryClient({
	defaultOptions: {
		queries: {
			staleTime: 1000 * 60 * 5, // 5 minutes
			retry: 1,
		},
	},
});
</script>

<svelte:head>
  <link rel="icon" href={favicon} />
</svelte:head>

<QueryClientProvider client={queryClient}>
	{@render children()}
</QueryClientProvider>

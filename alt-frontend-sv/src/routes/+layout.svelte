<script lang="ts">
import { onMount, setContext } from "svelte";
import "./layout.css";
import favicon from "$lib/assets/favicon.svg";
import { QueryClient, QueryClientProvider } from "@tanstack/svelte-query";
import { page } from "$app/state";
import { createAuthStore, AUTH_STORE_KEY } from "$lib/stores/auth.svelte";
import {
	createLoadingStore,
	LOADING_STORE_KEY,
} from "$lib/stores/loading.svelte";

const { children } = $props();

// Create SSR-safe store instances and inject via context
const auth = createAuthStore();
setContext(AUTH_STORE_KEY, auth);

const loadingStore = createLoadingStore();
setContext(LOADING_STORE_KEY, loadingStore);

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

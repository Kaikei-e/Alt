<script lang="ts">
import { onMount, setContext } from "svelte";
import "./layout.css";
import favicon from "$lib/assets/favicon.svg";
import {
	QueryCache,
	QueryClient,
	QueryClientProvider,
} from "@tanstack/svelte-query";
import { page, updated } from "$app/state";
import { createAuthStore, AUTH_STORE_KEY } from "$lib/stores/auth.svelte";
import {
	createLoadingStore,
	LOADING_STORE_KEY,
} from "$lib/stores/loading.svelte";
import {
	createConnectionRecoveryStore,
	CONNECTION_RECOVERY_KEY,
} from "$lib/stores/connection-recovery.svelte";
import {
	isNetworkFailureError,
	performGuardedReload,
} from "$lib/hooks/safari-connection-recovery";

const { children } = $props();

// Create SSR-safe store instances and inject via context
const auth = createAuthStore();
setContext(AUTH_STORE_KEY, auth);

const loadingStore = createLoadingStore();
setContext(LOADING_STORE_KEY, loadingStore);

// Safari connection recovery store for refetching after idle
const connectionRecovery = createConnectionRecoveryStore();
setContext(CONNECTION_RECOVERY_KEY, connectionRecovery);

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

// When the deployed version diverges from the loaded one, reload before
// the next chunk fetch lands on an evicted /_app/immutable/* hash — the
// failure mode that surfaces as "Cannot Open the Page" on iOS Safari.
let reloadOnUpdate = false;
$effect(() => {
	if (!updated.current || reloadOnUpdate) return;
	reloadOnUpdate = true;
	if (typeof window !== "undefined") {
		console.warn("[layout] new build detected — reloading");
		window.location.reload();
	}
});

// Create QueryClient for TanStack Query with Safari-friendly defaults.
// If a query still fails with a network error shortly after a connection
// recovery event, Safari is almost certainly holding a dead connection that
// only a fresh navigation will clear — do one guarded full reload.
const queryClient = new QueryClient({
	queryCache: new QueryCache({
		onError: (error) => {
			if (typeof window === "undefined") return;
			if (navigator?.onLine === false) return;
			if (!isNetworkFailureError(error)) return;
			if (!connectionRecovery.wasRecentlyRecovered(20_000)) return;
			performGuardedReload();
		},
	}),
	defaultOptions: {
		queries: {
			staleTime: 1000 * 60 * 5, // 5 minutes
			retry: 2,
			retryDelay: (attemptIndex) => Math.min(1000 * 2 ** attemptIndex, 10000),
			refetchOnWindowFocus: true,
			refetchOnReconnect: true,
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

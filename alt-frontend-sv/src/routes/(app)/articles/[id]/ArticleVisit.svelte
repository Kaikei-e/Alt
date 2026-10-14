<script lang="ts">
import { onMount } from "svelte";
import { trackHomeActionClient } from "#lib/api/client/knowledge_home.js";
import { useTrailOutcome } from "#lib/hooks/useTrailOutcome.svelte.js";
import { browser } from "$app/env";
import { page } from "$app/state";

interface Props {
	articleId: string;
	onVisit?: (articleId: string) => void;
}

const { articleId, onVisit }: Props = $props();

// Dwell measurement for a walked trail branch. The ?trail_proposal= handoff is
// the sole gate (D19): captured once at init of each keyed article visit and
// emitted by the hook on leave/destroy.
useTrailOutcome(
	page.url.searchParams.get("trail_proposal"),
	page.params.id ? `article:${page.params.id}` : null,
);

onMount(() => {
	if (!browser || !articleId) return;
	onVisit?.(articleId);
	void trackHomeActionClient(
		"open",
		`article:${articleId}`,
		JSON.stringify({ source: "article_page" }),
	);
});
</script>

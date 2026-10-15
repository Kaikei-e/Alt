<script lang="ts">
import { onMount, untrack } from "svelte";
import { trackHomeActionClient } from "#lib/api/client/knowledge_home.js";
import { useTrailOutcome } from "#lib/hooks/useTrailOutcome.svelte.js";
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
	`article:${untrack(() => articleId)}`,
);

onMount(() => {
	onVisit?.(articleId);
	void trackHomeActionClient(
		"open",
		`article:${articleId}`,
		JSON.stringify({ source: "article_page" }),
	);
});
</script>

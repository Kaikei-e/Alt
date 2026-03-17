<script lang="ts">
import { ArrowUpCircle } from "@lucide/svelte";
import type { SupersedeInfoData } from "$lib/connect/knowledge_home";

interface Props {
	info: SupersedeInfoData;
}

const { info }: Props = $props();

const label = $derived(() => {
	switch (info.state) {
		case "summary_updated": return "Summary updated";
		case "tags_updated": return "Tags updated";
		case "both_updated": return "Updated";
		default: return "Updated";
	}
});
</script>

<span
	class="inline-flex items-center gap-1 px-1.5 py-0.5 text-xs rounded border text-sky-400 border-sky-400/30 bg-sky-400/10"
	title={`Updated at ${info.supersededAt}`}
>
	<ArrowUpCircle class="h-3 w-3" />
	{label()}
</span>

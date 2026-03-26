<script lang="ts">
import { resolveRecallReason } from "./recall-reason-map";

interface Props {
	reasonType: string;
	description?: string;
}

const { reasonType, description }: Props = $props();
const display = $derived(resolveRecallReason(reasonType, description));
</script>

<span
	class="inline-flex items-center gap-1 px-1.5 py-0.5 text-xs rounded border {display.colorClass}"
>
	{display.label}
</span>

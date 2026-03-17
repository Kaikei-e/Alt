<script lang="ts">
import { resolveRecallReason } from "./recall-reason-map";

interface Props {
	reasonType: string;
}

const { reasonType }: Props = $props();
const display = $derived(resolveRecallReason(reasonType));
</script>

<span
	class="inline-flex items-center gap-1 px-1.5 py-0.5 text-xs rounded border {display.colorClass}"
>
	{display.label}
</span>

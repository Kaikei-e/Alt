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
	class="badge {display.colorClass}"
>
	{display.label}
</span>

<style>
	.badge {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		padding: 0.14rem 0.5rem;
		font-family: var(--font-mono);
		font-size: 0.62rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		line-height: 1.4;
		text-transform: uppercase;
		border-width: 1px;
		border-style: solid;
		white-space: nowrap;
	}
</style>

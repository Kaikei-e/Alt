<script lang="ts">
import LedgerFigure from "$lib/components/recap/job-status/LedgerFigure.svelte";

interface Props {
	title: string;
	value: string | number;
	subtitle?: string;
	delta?: number | null;
}

let { title, value, subtitle, delta = null }: Props = $props();
</script>

<LedgerFigure label={title} {value} {subtitle} {delta} />

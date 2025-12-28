<script lang="ts">
// @biome-ignore lint/style/useImportType: AccordionPrimitive is used as a component, not just a type
import type { Accordion as AccordionPrimitive } from "bits-ui";

let {
	ref = $bindable(null),
	value = $bindable(),
	...restProps
}: AccordionPrimitive.RootProps = $props();
</script>

<AccordionPrimitive.Root
	bind:ref
	bind:value={value as never}
	data-slot="accordion"
	{...restProps}
/>

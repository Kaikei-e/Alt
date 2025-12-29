<script lang="ts">
// @biome-ignore lint/style/useImportType: AccordionPrimitive is used as a component, not just a type
import { Accordion as AccordionPrimitive } from "bits-ui";
import { cn } from "$lib/utils.js";

let {
	ref = $bindable(null),
	class: className,
	...restProps
}: AccordionPrimitive.ItemProps = $props();
</script>

<AccordionPrimitive.Item
	bind:ref
	data-slot="accordion-item"
	class={cn("border-b last:border-b-0", className)}
	{...restProps}
/>

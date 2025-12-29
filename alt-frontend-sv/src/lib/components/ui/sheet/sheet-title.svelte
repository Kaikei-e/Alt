<script lang="ts">
// @biome-ignore lint/style/useImportType: SheetPrimitive is used as a component, not just a type
import { Dialog as SheetPrimitive } from "bits-ui";
import { cn } from "$lib/utils.js";

let {
	ref = $bindable(null),
	class: className,
	...restProps
}: SheetPrimitive.TitleProps = $props();
</script>

<SheetPrimitive.Title
	bind:ref
	data-slot="sheet-title"
	class={cn("text-foreground font-semibold", className)}
	{...restProps}
/>

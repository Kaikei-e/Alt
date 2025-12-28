<script lang="ts">
// @biome-ignore lint/style/useImportType: SheetPrimitive is used as a component, not just a type
import type { Dialog as SheetPrimitive } from "bits-ui";

let { ref = $bindable(null), ...restProps }: SheetPrimitive.TriggerProps =
	$props();
</script>

<SheetPrimitive.Trigger bind:ref data-slot="sheet-trigger" {...restProps} />

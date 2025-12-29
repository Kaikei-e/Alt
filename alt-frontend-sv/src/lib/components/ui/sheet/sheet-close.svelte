<script lang="ts">
// @biome-ignore lint/style/useImportType: SheetPrimitive is used as a component, not just a type
import { Dialog as SheetPrimitive } from "bits-ui";

let { ref = $bindable(null), ...restProps }: SheetPrimitive.CloseProps =
	$props();
</script>

<SheetPrimitive.Close bind:ref data-slot="sheet-close" {...restProps} />

<script lang="ts">
// @biome-ignore lint/style/useImportType: SheetPrimitive is used as a component, not just a type
import type { Dialog as SheetPrimitive } from "bits-ui";

let { open = $bindable(false), ...restProps }: SheetPrimitive.RootProps =
	$props();
</script>

<SheetPrimitive.Root bind:open {...restProps} />

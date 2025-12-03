<script lang="ts">
import type { Snippet } from "svelte";
import type { HTMLAttributes } from "svelte/elements";
import type { WithElementRef } from "$lib/utils.js";
import { cn } from "$lib/utils.js";

type CardHeaderProps = WithElementRef<HTMLAttributes<HTMLDivElement>> & {
	children?: Snippet;
};

let {
	class: className,
	ref = $bindable(null),
	children,
	...restProps
}: CardHeaderProps = $props();
</script>

<div
	bind:this={ref}
	class={cn("flex flex-col space-y-1.5 p-6", className)}
	{...restProps}
>
	{@render children?.()}
</div>


<script lang="ts">
import type { Snippet } from "svelte";
import type { HTMLAttributes } from "svelte/elements";
import type { WithElementRef } from "$lib/utils.js";
import { cn } from "$lib/utils.js";

type CardContentProps = WithElementRef<HTMLAttributes<HTMLDivElement>> & {
	children?: Snippet;
};

let {
	class: className,
	ref = $bindable(null),
	children,
	...restProps
}: CardContentProps = $props();
</script>

<div
	bind:this={ref}
	class={cn("p-6 pt-0 text-[var(--text-primary)]", className)}
	{...restProps}
>
	{@render children?.()}
</div>


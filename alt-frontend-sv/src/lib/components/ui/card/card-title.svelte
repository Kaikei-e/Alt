<script lang="ts">
import type { Snippet } from "svelte";
import type { HTMLAttributes } from "svelte/elements";
import type { WithElementRef } from "$lib/utils.js";
import { cn } from "$lib/utils.js";

type CardTitleProps = WithElementRef<HTMLAttributes<HTMLHeadingElement>> & {
	children?: Snippet;
};

let {
	class: className,
	ref = $bindable(null),
	children,
	...restProps
}: CardTitleProps = $props();
</script>

<h3
	bind:this={ref}
	class={cn("font-bold leading-none tracking-tight text-[var(--text-primary)]", className)}
	{...restProps}
>
	{@render children?.()}
</h3>


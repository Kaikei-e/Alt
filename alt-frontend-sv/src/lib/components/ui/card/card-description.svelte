<script lang="ts">
import type { Snippet } from "svelte";
import type { HTMLAttributes } from "svelte/elements";
import type { WithElementRef } from "$lib/utils.js";
import { cn } from "$lib/utils.js";

type CardDescriptionProps = WithElementRef<
	HTMLAttributes<HTMLParagraphElement>
> & {
	children?: Snippet;
};

let {
	class: className,
	ref = $bindable(null),
	children,
	...restProps
}: CardDescriptionProps = $props();
</script>

<p
	bind:this={ref}
	class={cn("text-sm text-[var(--text-muted)]", className)}
	{...restProps}
>
	{@render children?.()}
</p>


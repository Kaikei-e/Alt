<script lang="ts">
	import { cn } from "$lib/utils.js";
	import type { HTMLAttributes } from "svelte/elements";
	import type { WithElementRef } from "$lib/utils.js";
	import type { Snippet } from "svelte";

	type CardFooterProps = WithElementRef<HTMLAttributes<HTMLDivElement>> & {
		children?: Snippet;
	};

	let {
		class: className,
		ref = $bindable(null),
		children,
		...restProps
	}: CardFooterProps = $props();
</script>

<div
	bind:this={ref}
	class={cn("flex items-center p-6 pt-0 text-[var(--text-primary)]", className)}
	{...restProps}
>
	{@render children?.()}
</div>


<script lang="ts">
	import { cn } from "$lib/utils.js";
	import type { HTMLAttributes } from "svelte/elements";
	import type { WithElementRef } from "$lib/utils.js";
	import type { Snippet } from "svelte";

	type CardProps = WithElementRef<HTMLAttributes<HTMLDivElement>> & {
		children?: Snippet;
	};

	let {
		class: className,
		ref = $bindable(null),
		children,
		...restProps
	}: CardProps = $props();
</script>

<div
	bind:this={ref}
	class={cn("rounded-none border-2 bg-[var(--surface-bg)] border-[var(--surface-border)] text-[var(--text-primary)] shadow-[var(--shadow-sm)]", className)}
	{...restProps}
>
	{@render children?.()}
</div>


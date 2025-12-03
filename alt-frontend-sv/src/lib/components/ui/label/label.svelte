<script lang="ts">
	import { cn } from "$lib/utils.js";
	import type { HTMLLabelAttributes } from "svelte/elements";
	import type { WithElementRef } from "$lib/utils.js";
	import type { Snippet } from "svelte";

	type LabelProps = WithElementRef<HTMLLabelAttributes> & {
		children?: Snippet;
	};

	let {
		class: className,
		ref = $bindable(null),
		children,
		...restProps
	}: LabelProps = $props();
</script>

<label
	bind:this={ref}
	class={cn(
		"text-sm font-medium leading-none text-[var(--text-primary)] peer-disabled:cursor-not-allowed peer-disabled:opacity-70",
		className
	)}
	{...restProps}
>
	{@render children?.()}
</label>


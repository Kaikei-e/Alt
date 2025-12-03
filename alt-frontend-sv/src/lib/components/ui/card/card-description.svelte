<script lang="ts">
	import { cn } from "$lib/utils.js";
	import type { HTMLAttributes } from "svelte/elements";
	import type { WithElementRef } from "$lib/utils.js";
	import type { Snippet } from "svelte";

	type CardDescriptionProps = WithElementRef<HTMLAttributes<HTMLParagraphElement>> & {
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


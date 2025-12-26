<script lang="ts">
import type { HTMLInputAttributes } from "svelte/elements";
import type { WithElementRef } from "$lib/utils.js";
import { cn } from "$lib/utils.js";

type InputProps = WithElementRef<HTMLInputAttributes>;

let {
	class: className,
	type = "text",
	ref = $bindable(null),
	value = $bindable(),
	...restProps
}: InputProps = $props();
</script>

<input
	bind:this={ref}
	bind:value={value}
	type={type}
	class={cn(
		"flex h-9 w-full rounded-[12px] border-2 bg-[var(--surface-bg)] px-4 py-1 text-base text-[var(--text-primary)] shadow-sm transition-all file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-[var(--text-primary)] placeholder:text-[var(--text-muted)] focus-visible:outline-none focus-visible:border-[var(--alt-primary)] focus-visible:shadow-[var(--shadow-sm)] hover:border-[var(--surface-border)] disabled:cursor-not-allowed disabled:opacity-60 disabled:bg-[var(--surface-bg)] disabled:border-[var(--surface-border)]",
		className
	)}
	style="border-color: var(--surface-border);"
	{...restProps}
/>


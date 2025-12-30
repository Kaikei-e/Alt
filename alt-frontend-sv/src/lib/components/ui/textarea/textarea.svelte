<script lang="ts">
import type { HTMLTextareaAttributes } from "svelte/elements";
import type { WithElementRef } from "$lib/utils.js";
import { cn } from "$lib/utils.js";

type TextareaProps = WithElementRef<HTMLTextareaAttributes>;

let {
	class: className,
	ref = $bindable(null),
	value = $bindable(),
	...restProps
}: TextareaProps = $props();
</script>

<textarea
	bind:this={ref}
	bind:value={value}
	class={cn(
		"flex min-h-[60px] w-full rounded-[12px] border-2 bg-[var(--surface-bg)] px-4 py-2 text-base text-[var(--text-primary)] shadow-sm transition-all placeholder:text-[var(--text-muted)] focus-visible:outline-none focus-visible:border-[var(--alt-primary)] focus-visible:shadow-[var(--shadow-sm)] hover:border-[var(--surface-border)] disabled:cursor-not-allowed disabled:opacity-60 disabled:bg-[var(--surface-bg)] disabled:border-[var(--surface-border)] resize-y",
		className
	)}
	style="border-color: var(--surface-border);"
	{...restProps}
></textarea>

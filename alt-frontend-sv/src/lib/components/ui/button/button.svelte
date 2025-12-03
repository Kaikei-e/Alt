<script lang="ts" module>
import type {
	HTMLAnchorAttributes,
	HTMLButtonAttributes,
} from "svelte/elements";
import { tv, type VariantProps } from "tailwind-variants";
import type { WithElementRef } from "$lib/utils.js";

export const buttonVariants = tv({
	base: "focus-visible:outline-none inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-none text-base font-bold outline-none transition-all disabled:pointer-events-none disabled:opacity-60 aria-disabled:pointer-events-none aria-disabled:opacity-60 [&_svg:not([class*='size-'])]:size-4 [&_svg]:pointer-events-none [&_svg]:shrink-0",
	variants: {
		variant: {
			default:
				"bg-[var(--surface-bg)] text-[var(--text-primary)] border-2 border-[var(--alt-primary)] shadow-[var(--shadow-sm)] hover:bg-[var(--alt-primary)] hover:text-white hover:shadow-[var(--shadow-md)]",
			destructive:
				"bg-[var(--surface-bg)] text-[#dc2626] border-2 border-[#dc2626] shadow-[var(--shadow-sm)] hover:bg-[#dc2626] hover:text-white hover:shadow-[var(--shadow-md)]",
			outline:
				"bg-[var(--surface-bg)] text-[var(--text-primary)] border-2 border-[var(--surface-border)] shadow-[var(--shadow-sm)] hover:bg-[var(--surface-hover)] hover:border-[var(--alt-primary)]",
			secondary:
				"bg-[var(--surface-bg)] text-[var(--text-secondary)] border-2 border-[var(--alt-secondary)] shadow-[var(--shadow-sm)] hover:bg-[var(--alt-secondary)] hover:text-white",
			ghost:
				"bg-transparent text-[var(--text-primary)] border-2 border-transparent hover:bg-[var(--surface-hover)] hover:border-[var(--surface-border)]",
			link: "text-[var(--alt-primary)] underline-offset-4 hover:underline border-0 bg-transparent shadow-none",
		},
		size: {
			default: "h-9 px-4 py-2 has-[>svg]:px-3",
			sm: "h-8 gap-1.5 px-3 has-[>svg]:px-2.5 text-sm",
			lg: "h-10 px-6 has-[>svg]:px-4 text-lg",
			icon: "size-9",
			"icon-sm": "size-8",
			"icon-lg": "size-10",
		},
	},
	defaultVariants: {
		variant: "default",
		size: "default",
	},
});

export type ButtonVariant = VariantProps<typeof buttonVariants>["variant"];
export type ButtonSize = VariantProps<typeof buttonVariants>["size"];

export type ButtonProps = WithElementRef<HTMLButtonAttributes> &
	WithElementRef<HTMLAnchorAttributes> & {
		variant?: ButtonVariant;
		size?: ButtonSize;
	};
</script>

<script lang="ts">
	let {
		class: className,
		variant = "default",
		size = "default",
		ref = $bindable(null),
		href = undefined,
		type = "button",
		disabled,
		children,
		...restProps
	}: ButtonProps = $props();
	import { cn } from "$lib/utils.js";
</script>

{#if href}
	<a
		bind:this={ref}
		data-slot="button"
		class={cn(buttonVariants({ variant, size }), className)}
		href={disabled ? undefined : href}
		aria-disabled={disabled}
		role={disabled ? "link" : undefined}
		tabindex={disabled ? -1 : undefined}
		{...restProps}
	>
		{@render children?.()}
	</a>
{:else}
	<button
		bind:this={ref}
		data-slot="button"
		class={cn(buttonVariants({ variant, size }), className)}
		{type}
		{disabled}
		{...restProps}
	>
		{@render children?.()}
	</button>
{/if}

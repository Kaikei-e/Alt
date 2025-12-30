<script lang="ts">
	import { Loader2 } from "@lucide/svelte";
	import { cn } from "$lib/utils";
	import altLogo from "$lib/assets/augur1.png";

	interface Props {
		class?: string;
		reduceMotion?: boolean;
	}

	const { class: className, reduceMotion = false }: Props = $props();
</script>

<div
	class={cn(
		"fixed inset-0 z-50 flex items-center justify-center bg-[var(--surface-bg)]",
		className
	)}
	data-testid="system-loader"
	role="status"
	aria-label="Loading Alt"
>
	<div class="flex flex-col items-center gap-6">
		<img src={altLogo} alt="Alt Logo" class="w-16 h-16" />

		<p class="text-lg font-medium text-[var(--text-primary)]">
			Loading Alt
		</p>

		<Loader2
			class={cn("h-8 w-8 text-[var(--alt-primary)]", !reduceMotion && "animate-spin")}
		/>
	</div>
</div>

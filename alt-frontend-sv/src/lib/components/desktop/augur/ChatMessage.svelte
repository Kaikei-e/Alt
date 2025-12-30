<script lang="ts">
	import { User, BirdIcon } from "@lucide/svelte";
	import { cn } from "$lib/utils";

	interface Props {
		message: string;
		role: "user" | "assistant";
		timestamp?: string;
	}

	let { message, role, timestamp }: Props = $props();

	let isUser = $derived(role === "user");
</script>

<div
	class={cn(
		"flex gap-3 mb-4",
		isUser ? "justify-end" : "justify-start"
	)}
>
	{#if !isUser}
		<div class="flex-shrink-0 w-8 h-8 rounded-full bg-[var(--accent-primary)] flex items-center justify-center">
			<BirdIcon class="h-4 w-4 text-white" />
		</div>
	{/if}

	<div
		class={cn(
			"max-w-[70%] p-3 text-sm",
			isUser
				? "bg-[var(--accent-primary)] text-white"
				: "bg-[var(--surface-hover)] text-[var(--text-primary)]"
		)}
	>
		<p class="whitespace-pre-wrap break-words leading-relaxed">{message}</p>
		{#if timestamp}
			<p
				class={cn(
					"text-xs mt-1",
					isUser ? "text-white/70" : "text-[var(--text-muted)]"
				)}
			>
				{timestamp}
			</p>
		{/if}
	</div>

	{#if isUser}
		<div class="flex-shrink-0 w-8 h-8 rounded-full bg-[var(--surface-border)] flex items-center justify-center">
			<User class="h-4 w-4 text-[var(--text-secondary)]" />
		</div>
	{/if}
</div>

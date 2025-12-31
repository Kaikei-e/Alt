<script lang="ts">
	import { User } from "@lucide/svelte";
	import { cn } from "$lib/utils";
	import { parseMarkdown } from "$lib/utils/simpleMarkdown";
	import augurAvatar from "$lib/assets/augur-chat.webp";

	type Citation = {
		URL: string;
		Title: string;
		PublishedAt?: string;
		Score?: number;
	};

	type Props = {
		message: string;
		role: "user" | "assistant";
		timestamp?: string;
		citations?: Citation[];
	};

	let { message, role, timestamp, citations }: Props = $props();

	let isUser = $derived(role === "user");
</script>

<div
	class={cn(
		"flex gap-3 mb-4",
		isUser ? "justify-end" : "justify-start"
	)}
>
	{#if !isUser}
		<div class="flex-shrink-0 w-8 h-8 rounded-full overflow-hidden bg-muted mt-1 shadow-sm border border-border/50">
			<img src={augurAvatar} alt="Augur" class="w-full h-full object-cover" />
		</div>
	{/if}

	<div class="max-w-[70%] flex flex-col gap-2">
		<div
			class={cn(
				"p-3 text-sm rounded-2xl shadow-sm",
				isUser
					? "bg-primary text-primary-foreground rounded-br-none"
					: "bg-muted/50 text-foreground rounded-bl-none border border-border/50"
			)}
		>
			{#if isUser}
				<p class="whitespace-pre-wrap break-words leading-relaxed">{message}</p>
			{:else}
				<div class="whitespace-pre-wrap break-words leading-relaxed">
					{@html parseMarkdown(message)}
				</div>
			{/if}
			{#if timestamp}
				<p
					class={cn(
						"text-xs mt-1 opacity-70"
					)}
				>
					{timestamp}
				</p>
			{/if}
		</div>

		{#if !isUser && citations && citations.length > 0}
			<div class="bg-muted/30 border border-border/50 rounded-lg p-3 text-xs text-muted-foreground">
				<p class="font-semibold mb-2">Sources:</p>
				<ul class="space-y-2">
					{#each citations as cite, i}
						<li>
							<a
								href={cite.URL}
								target="_blank"
								rel="noopener noreferrer"
								class="hover:text-foreground flex gap-2 group items-start"
							>
								<span class="font-mono opacity-70 shrink-0 mt-0.5">[{i + 1}]</span>
								<span class="underline decoration-muted-foreground/50 group-hover:decoration-foreground underline-offset-4 break-words leading-relaxed">
									{cite.Title || 'Untitled Source'}
								</span>
							</a>
						</li>
					{/each}
				</ul>
			</div>
		{/if}
	</div>

	{#if isUser}
		<div class="flex-shrink-0 w-8 h-8 rounded-full bg-muted mt-1 shadow-sm border border-border/50 flex items-center justify-center">
			<div class="w-full h-full bg-primary/20 flex items-center justify-center rounded-full">
				<User class="h-4 w-4 text-primary" />
			</div>
		</div>
	{/if}
</div>

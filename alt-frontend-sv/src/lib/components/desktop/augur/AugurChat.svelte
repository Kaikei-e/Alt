<script lang="ts">
	import { onMount } from "svelte";
	import { Loader2 } from "@lucide/svelte";
	import ChatMessage from "./ChatMessage.svelte";
	import ChatInput from "./ChatInput.svelte";

	type Message = {
		id: string;
		message: string;
		role: "user" | "assistant";
		timestamp: string;
	};

	let messages = $state<Message[]>([
		{
			id: "welcome",
			message: "Hello! I'm Augur. Ask me anything about your RSS feeds.",
			role: "assistant",
			timestamp: new Date().toLocaleTimeString(),
		},
	]);

	let isLoading = $state(false);
	let chatContainer: HTMLDivElement;

	function scrollToBottom() {
		if (chatContainer) {
			setTimeout(() => {
				chatContainer.scrollTop = chatContainer.scrollHeight;
			}, 100);
		}
	}

	async function handleSend(messageText: string) {
		// Add user message
		const userMessage: Message = {
			id: `user-${Date.now()}`,
			message: messageText,
			role: "user",
			timestamp: new Date().toLocaleTimeString(),
		};

		messages = [...messages, userMessage];
		scrollToBottom();

		isLoading = true;

		try {
			// TODO: Call actual Augur API here
			// For now, simulate a response
			await new Promise((resolve) => setTimeout(resolve, 1000));

			const assistantMessage: Message = {
				id: `assistant-${Date.now()}`,
				message: "I'm a placeholder response. Augur API integration is pending.",
				role: "assistant",
				timestamp: new Date().toLocaleTimeString(),
			};

			messages = [...messages, assistantMessage];
			scrollToBottom();
		} catch (error) {
			const errorMessage: Message = {
				id: `error-${Date.now()}`,
				message: `Error: ${error instanceof Error ? error.message : "Unknown error"}`,
				role: "assistant",
				timestamp: new Date().toLocaleTimeString(),
			};

			messages = [...messages, errorMessage];
		} finally {
			isLoading = false;
		}
	}

	onMount(() => {
		scrollToBottom();
	});
</script>

<div class="flex flex-col h-[calc(100vh-12rem)] max-w-4xl mx-auto border border-[var(--surface-border)] bg-white">
	<!-- Chat messages -->
	<div bind:this={chatContainer} class="flex-1 overflow-y-auto p-6">
		{#each messages as msg (msg.id)}
			<ChatMessage message={msg.message} role={msg.role} timestamp={msg.timestamp} />
		{/each}

		{#if isLoading}
			<div class="flex gap-3 mb-4">
				<div class="flex-shrink-0 w-8 h-8 rounded-full bg-[var(--accent-primary)] flex items-center justify-center">
					<Loader2 class="h-4 w-4 text-white animate-spin" />
				</div>
				<div class="bg-[var(--surface-hover)] p-3 text-sm">
					<p class="text-[var(--text-secondary)]">Augur is thinking...</p>
				</div>
			</div>
		{/if}
	</div>

	<!-- Input -->
	<ChatInput onSend={handleSend} disabled={isLoading} />
</div>

<script lang="ts">
	import { onMount, tick } from "svelte";
	import { Loader2 } from "@lucide/svelte";
	import ChatMessage from "./ChatMessage.svelte";
	import ChatInput from "./ChatInput.svelte";
	import { createClientTransport, streamAugurChat, type AugurCitation } from "$lib/connect";
	import augurAvatar from "$lib/assets/augur-chat.webp";

	type Citation = {
		URL: string;
		Title: string;
		PublishedAt?: string;
		Score?: number;
	};

	type Message = {
		id: string;
		message: string;
		role: "user" | "assistant";
		timestamp: string;
		citations?: Citation[];
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
	let currentAbortController: AbortController | null = null;

	async function scrollToBottom() {
		await tick();
		if (chatContainer) {
			setTimeout(() => {
				chatContainer.scrollTop = chatContainer.scrollHeight;
			}, 100);
		}
	}

	/**
	 * Convert AugurCitation from Connect-RPC to component Citation format
	 */
	function convertCitations(citations: AugurCitation[]): Citation[] {
		return citations.map((c) => ({
			URL: c.url,
			Title: c.title,
			PublishedAt: c.publishedAt,
		}));
	}

	async function handleSend(messageText: string) {
		// Cancel any ongoing stream
		if (currentAbortController) {
			currentAbortController.abort();
			currentAbortController = null;
		}

		// Add user message
		const userMessage: Message = {
			id: `user-${Date.now()}`,
			message: messageText,
			role: "user",
			timestamp: new Date().toLocaleTimeString(),
		};

		messages = [...messages, userMessage];
		await scrollToBottom();

		isLoading = true;

		// Add placeholder for assistant message
		messages = [...messages, {
			id: `assistant-${Date.now()}`,
			message: "",
			role: "assistant",
			timestamp: new Date().toLocaleTimeString(),
		}];
		const currentAssistantMessageIndex = messages.length - 1;

		// Throttling state for delta updates
		let bufferedContent = "";
		let lastUpdateTime = 0;
		const THROTTLE_MS = 50;

		try {
			const transport = createClientTransport();

			// Build message history (excluding the empty placeholder)
			const chatHistory = messages.slice(0, -1).map((m) => ({
				role: m.role as "user" | "assistant",
				content: m.message,
			}));

			currentAbortController = streamAugurChat(
				transport,
				{ messages: chatHistory },
				// onDelta: text chunk received
				(text) => {
					bufferedContent += text;

					const now = Date.now();
					if (now - lastUpdateTime > THROTTLE_MS) {
						messages[currentAssistantMessageIndex] = {
							...messages[currentAssistantMessageIndex],
							message: bufferedContent,
						};
						lastUpdateTime = now;
					}
				},
				// onMeta: citations received
				(citations) => {
					messages[currentAssistantMessageIndex] = {
						...messages[currentAssistantMessageIndex],
						citations: convertCitations(citations),
					};
				},
				// onComplete: streaming finished
				(result) => {
					// Ensure final content is rendered
					messages[currentAssistantMessageIndex] = {
						...messages[currentAssistantMessageIndex],
						message: result.answer || bufferedContent,
						citations: result.citations.length > 0
							? convertCitations(result.citations)
							: messages[currentAssistantMessageIndex].citations,
					};
					isLoading = false;
					currentAbortController = null;
					scrollToBottom();
				},
				// onFallback: insufficient context
				(code) => {
					messages[currentAssistantMessageIndex] = {
						...messages[currentAssistantMessageIndex],
						message: "I apologize, but I couldn't find enough information in my knowledge base to answer that properly.",
					};
					isLoading = false;
					currentAbortController = null;
					scrollToBottom();
				},
				// onError: error occurred
				(error) => {
					console.error("Chat error:", error);
					messages[currentAssistantMessageIndex] = {
						...messages[currentAssistantMessageIndex],
						message: `Error: ${error.message}. Please try again.`,
					};
					isLoading = false;
					currentAbortController = null;
					scrollToBottom();
				},
			);
		} catch (error) {
			console.error("Chat error:", error);
			messages[currentAssistantMessageIndex] = {
				...messages[currentAssistantMessageIndex],
				message: `Error: ${error instanceof Error ? error.message : "Unknown error"}. Please try again.`,
			};
			isLoading = false;
			await scrollToBottom();
		}
	}

	onMount(() => {
		scrollToBottom();

		// Cleanup on unmount
		return () => {
			if (currentAbortController) {
				currentAbortController.abort();
			}
		};
	});
</script>

<div class="flex flex-col h-[calc(100vh-12rem)] max-w-4xl mx-auto border border-border bg-background rounded-lg overflow-hidden">
	<!-- Chat messages -->
	<div bind:this={chatContainer} class="flex-1 overflow-y-auto p-6">
		{#each messages as msg (msg.id)}
			<ChatMessage
				message={msg.message}
				role={msg.role}
				timestamp={msg.timestamp}
				citations={msg.citations}
			/>
		{/each}

		{#if isLoading}
			<div class="flex gap-3 mb-4">
				<div class="flex-shrink-0 w-8 h-8 rounded-full overflow-hidden bg-muted mt-1 shadow-sm border border-border/50 relative">
					<img src={augurAvatar} alt="Augur" class="w-full h-full object-cover" />
					<div class="absolute inset-0 bg-background/40 flex items-center justify-center">
						<Loader2 class="h-4 w-4 text-primary animate-spin" />
					</div>
				</div>
				<div class="bg-muted/50 p-3 text-sm rounded-2xl rounded-bl-none shadow-sm border border-border/50">
					<p class="text-muted-foreground">Augur is thinking...</p>
				</div>
			</div>
		{/if}
	</div>

	<!-- Input -->
	<ChatInput onSend={handleSend} disabled={isLoading} />
</div>

<script lang="ts">
	import { onMount, tick } from "svelte";
	import { Loader2 } from "@lucide/svelte";
	import { base } from "$app/paths";
	import ChatMessage from "./ChatMessage.svelte";
	import ChatInput from "./ChatInput.svelte";
	import { processAugurStreamingText } from "$lib/utils/streamingRenderer";
	import augurAvatar from "$lib/assets/augur-mobile.png";

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

	async function scrollToBottom() {
		await tick();
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

		try {
			const response = await fetch(`${base}/api/v1/augur/chat`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({
					messages: messages.slice(0, -1).map(m => ({
						role: m.role,
						content: m.message,
					})),
				}),
			});

			if (!response.ok) throw new Error("Failed to send message");
			if (!response.body) throw new Error("No response body");

			const reader = response.body.getReader();

			// Throttling state
			let bufferedContent = "";
			let lastUpdateTime = 0;
			const THROTTLE_MS = 50;

			await processAugurStreamingText(
				reader,
				(text) => {
					// Accumulate text but don't update state immediately
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
				{
					tick: async () => {
						// Optional: custom tick if needed
					},
					typewriter: false,
					onMetadata: (meta: any) => {
						if (meta && (meta.Contexts || meta.Citations)) {
							// Prioritize Citations (final), fallback to Contexts (initial)
							const rawSources = meta.Citations || meta.Contexts || [];
							const cleanCitations: Citation[] = rawSources.map((s: any) => ({
								URL: s.URL,
								Title: s.Title,
								PublishedAt: s.PublishedAt,
								Score: s.Score,
							}));

							messages[currentAssistantMessageIndex] = {
								...messages[currentAssistantMessageIndex],
								citations: cleanCitations,
							};
						} else if (meta && meta.fallback) {
							// Handle fallback (insufficient evidence)
							bufferedContent = "I apologize, but I couldn't find enough information in my knowledge base to answer that properly.";
							messages[currentAssistantMessageIndex] = {
								...messages[currentAssistantMessageIndex],
								message: bufferedContent,
							};
						}
					},
					onComplete: () => {
						// Ensure final content is rendered
						if (messages[currentAssistantMessageIndex].message !== bufferedContent) {
							messages[currentAssistantMessageIndex] = {
								...messages[currentAssistantMessageIndex],
								message: bufferedContent,
							};
						}
					},
				},
			);
		} catch (error) {
			console.error("Chat error:", error);
			messages[currentAssistantMessageIndex] = {
				...messages[currentAssistantMessageIndex],
				message: `Error: ${error instanceof Error ? error.message : "Unknown error"}. Please try again.`,
			};
		} finally {
			isLoading = false;
			await scrollToBottom();
		}
	}

	onMount(() => {
		scrollToBottom();
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

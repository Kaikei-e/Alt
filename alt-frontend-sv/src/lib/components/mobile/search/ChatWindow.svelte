<script lang="ts">
import { Loader, SendHorizontal } from "@lucide/svelte";
import { tick } from "svelte";
import augurAvatar from "$lib/assets/augur-chat.webp";
import augurPlaceholder from "$lib/assets/augur-placeholder.webp";
import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
import { Button } from "$lib/components/ui/button";
import { Input } from "$lib/components/ui/input";
import { parseMarkdown } from "$lib/utils/simpleMarkdown";
import { createClientTransport, streamAugurChat, type AugurCitation } from "$lib/connect";

type Citation = {
	url: string;
	title: string;
	publishedAt: string;
};

type Message = {
	role: "user" | "assistant";
	content: string;
	citations?: Citation[];
};

// State
let messages: Message[] = $state([]);
let inputValue = $state("");
let isLoading = $state(false);
let messagesEndRef: HTMLDivElement;

// Auto-scroll to bottom
const scrollToBottom = async () => {
	await tick();
	if (messagesEndRef) {
		messagesEndRef.scrollIntoView({ behavior: "smooth" });
	}
};

const handleSubmit = async () => {
	if (!inputValue.trim() || isLoading) return;

	const userMessage = inputValue.trim();
	inputValue = "";

	// Add user message
	messages = [...messages, { role: "user", content: userMessage }];
	await scrollToBottom();

	isLoading = true;

	// Add placeholder for assistant message
	messages = [...messages, { role: "assistant", content: "" }];
	let currentAssistantMessageIndex = messages.length - 1;

	// Throttling state
	let bufferedContent = "";
	let lastUpdateTime = 0;
	const THROTTLE_MS = 50;

	try {
		const transport = createClientTransport();

		// Prepare messages for Connect-RPC (exclude empty placeholder)
		const chatMessages = messages.slice(0, -1).map((m) => ({
			role: m.role,
			content: m.content,
		}));

		streamAugurChat(
			transport,
			{ messages: chatMessages },
			// onDelta: text chunks
			(text) => {
				bufferedContent += text;

				const now = Date.now();
				if (now - lastUpdateTime > THROTTLE_MS) {
					const currentMsg = messages[currentAssistantMessageIndex];
					messages[currentAssistantMessageIndex] = {
						...currentMsg,
						content: bufferedContent,
					};
					lastUpdateTime = now;
				}
			},
			// onThinking: not displayed in UI
			undefined,
			// onMeta: citations
			(citations: AugurCitation[]) => {
				const cleanCitations: Citation[] = citations.map((c) => ({
					url: c.url,
					title: c.title,
					publishedAt: c.publishedAt,
				}));

				const currentMsg = messages[currentAssistantMessageIndex];
				messages[currentAssistantMessageIndex] = {
					...currentMsg,
					citations: cleanCitations,
				};
			},
			// onComplete: final result
			(result) => {
				// Ensure final content is rendered
				const currentMsg = messages[currentAssistantMessageIndex];
				messages[currentAssistantMessageIndex] = {
					...currentMsg,
					content: result.answer,
					citations: result.citations.map((c) => ({
						url: c.url,
						title: c.title,
						publishedAt: c.publishedAt,
					})),
				};
				isLoading = false;
				scrollToBottom();
			},
			// onFallback: insufficient evidence
			(code) => {
				const fallbackMessage =
					"I apologize, but I couldn't find enough information in my knowledge base to answer that properly.";
				const currentMsg = messages[currentAssistantMessageIndex];
				messages[currentAssistantMessageIndex] = {
					...currentMsg,
					content: fallbackMessage,
				};
				isLoading = false;
				scrollToBottom();
			},
			// onError: error handling
			(error) => {
				console.error("Chat error:", error);
				const currentMsg = messages[currentAssistantMessageIndex];
				messages[currentAssistantMessageIndex] = {
					...currentMsg,
					content: "Sorry, something went wrong. Please try again.",
				};
				isLoading = false;
				scrollToBottom();
			},
		);
	} catch (error) {
		console.error("Chat error:", error);
		const currentMsg = messages[currentAssistantMessageIndex];
		messages[currentAssistantMessageIndex] = {
			...currentMsg,
			content: "Sorry, something went wrong. Please try again.",
		};
		isLoading = false;
		await scrollToBottom();
	}
};
</script>

<div class="flex flex-col h-full bg-background relative">
  <!-- Messages Area -->
  <div class="flex-1 overflow-y-auto p-4 space-y-4 pb-20">
    {#if messages.length === 0}
      <div class="flex flex-col items-center justify-center h-full text-muted-foreground opacity-50">
        <img src={augurPlaceholder} alt="Augur" class="w-32 h-32 mb-4 rounded-full opacity-50 grayscale" />
        <p>Ask Augur anything...</p>
      </div>
    {/if}

    {#each messages as message}
      <div class="flex w-full {message.role === 'user' ? 'justify-end' : 'justify-start'}">
        <div class="flex max-w-[85%] gap-2 {message.role === 'user' ? 'flex-row-reverse' : 'flex-row'}">

          <!-- Avatar -->
          <div class="flex-shrink-0 w-8 h-8 rounded-full overflow-hidden bg-muted mt-1 shadow-sm border border-border/50">
             {#if message.role === 'assistant'}
                <img src={augurAvatar} alt="Augur" class="w-full h-full object-cover" />
             {:else}
                <div class="w-full h-full bg-primary/20 flex items-center justify-center text-xs font-bold text-primary">
                    You
                </div>
             {/if}
          </div>

          <!-- Message Bubble -->
          <div class="flex flex-col gap-2 max-w-full">
            <div class="
                p-3 rounded-2xl text-sm leading-relaxed shadow-sm break-words [overflow-wrap:anywhere]
                {message.role === 'user'
                ? 'bg-primary text-primary-foreground rounded-br-none'
                : 'bg-muted/50 text-foreground rounded-bl-none border border-border/50'}
            ">
                {#if message.role === 'assistant' && !message.content && isLoading && !message.citations}
                    <span class="flex gap-1 items-center h-5">
                        <span class="w-1.5 h-1.5 bg-current rounded-full animate-bounce delay-0"></span>
                        <span class="w-1.5 h-1.5 bg-current rounded-full animate-bounce delay-150"></span>
                        <span class="w-1.5 h-1.5 bg-current rounded-full animate-bounce delay-300"></span>
                    </span>
                {:else}
                <div class="text-foreground">
                    {#if message.role === 'assistant'}
                        {@html parseMarkdown(message.content)}
                    {:else}
                        <div class="whitespace-pre-wrap">{message.content}</div>
                    {/if}
                </div>
                {/if}
            </div>

            <!-- Citations -->
            {#if message.role === 'assistant' && message.citations && message.citations.length > 0}
                <div class="bg-muted/30 border border-border/50 rounded-lg p-3 text-xs text-muted-foreground ml-1 mt-1">
                    <p class="font-semibold mb-2">Sources:</p>
                    <ul class="space-y-2">
                        {#each message.citations as cite, i}
                            <li>
                                <a href={cite.url} target="_blank" rel="noopener noreferrer" class="hover:text-foreground flex gap-2 group items-start">
                                    <span class="font-mono opacity-70 shrink-0 mt-0.5">[{i + 1}]</span>
                                    <span class="underline decoration-muted-foreground/50 group-hover:decoration-foreground underline-offset-4 break-words leading-relaxed">
                                        {cite.title || 'Untitled Source'}
                                    </span>
                                </a>
                            </li>
                        {/each}
                    </ul>
                </div>
            {/if}
          </div>

        </div>
      </div>
    {/each}
    <div bind:this={messagesEndRef}></div>
  </div>

  <FloatingMenu class="bottom-24" />

  <!-- Input Area -->
  <div class="p-3 border-t bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60 fixed bottom-0 left-0 right-0 z-10 w-full md:max-w-md mx-auto">
     <form
        class="flex gap-2 items-end max-w-4xl mx-auto"
        onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}
    >
        <Input
            bind:value={inputValue}
            placeholder="Type your message..."
            class="min-h-[44px] rounded-full border-border/50 bg-muted/30 focus-visible:ring-offset-0 focus-visible:ring-1"
            disabled={isLoading}
        />
        <Button
            type="submit"
            size="icon"
            class="rounded-full h-11 w-11 shrink-0 shadow-sm"
            disabled={!inputValue.trim() || isLoading}
        >
            {#if isLoading}
                <Loader class="h-5 w-5 animate-spin" />
            {:else}
                <SendHorizontal class="h-5 w-5 ml-0.5" />
            {/if}
            <span class="sr-only">Send</span>
        </Button>
     </form>
  </div>
</div>

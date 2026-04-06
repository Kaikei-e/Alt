<script lang="ts">
import { tick } from "svelte";
import { FileText, Loader2, RotateCcw, Shuffle } from "@lucide/svelte";
import * as Sheet from "$lib/components/ui/sheet";
import ChatMessage from "$lib/components/desktop/augur/ChatMessage.svelte";
import ChatInput from "$lib/components/desktop/augur/ChatInput.svelte";
import { useAugurPane } from "$lib/hooks/useAugurPane.svelte";
import { useViewport } from "$lib/stores/viewport.svelte";
import { buildAugurInitialMessage } from "$lib/utils/augur-entry";
import { pickSuggestions } from "./ask-suggestions";
import augurAvatar from "$lib/assets/augur-chat.webp";

interface Props {
	open: boolean;
	scopeTitle: string;
	scopeContext?: string;
	scopeArticleId?: string;
	scopeTags?: string[];
	onClose: () => void;
}

const {
	open,
	scopeTitle,
	scopeContext,
	scopeArticleId,
	scopeTags,
	onClose,
}: Props = $props();

const { isDesktop } = useViewport();
const pane = useAugurPane();

let phase = $state<"ask" | "chat">("ask");
let question = $state("");
let chatContainer: HTMLDivElement | undefined = $state();
let shuffleCount = $state(0);

const suggestions = $derived(pickSuggestions(scopeTags, shuffleCount));
const sheetSide = $derived<"right" | "bottom">(isDesktop ? "right" : "bottom");

// Reset state when sheet closes
$effect(() => {
	if (!open) {
		phase = "ask";
		question = "";
		shuffleCount = 0;
		pane.reset();
	}
});

// Auto-scroll when messages update
$effect(() => {
	const _len = pane.messages.length;
	const lastMsg = pane.messages.at(-1);
	const _content = lastMsg?.message;
	if (chatContainer) {
		tick().then(() => {
			if (chatContainer) {
				chatContainer.scrollTop = chatContainer.scrollHeight;
			}
		});
	}
});

function handleAskSubmit() {
	const trimmed = question.trim();
	if (!trimmed) return;

	phase = "chat";
	const initialMessage = buildAugurInitialMessage(
		trimmed,
		scopeContext,
		scopeArticleId,
	);
	tick().then(() => pane.sendMessage(initialMessage));
}

// Retry: detect if last assistant message is an error/fallback (not loading)
const canRetry = $derived.by(() => {
	if (pane.isLoading || pane.messages.length < 2) return false;
	const last = pane.messages.at(-1);
	if (!last || last.role !== "assistant") return false;
	const msg = last.message;
	return (
		msg.startsWith("Error:") ||
		msg.includes("Not enough indexed content") ||
		msg.includes("couldn't find enough information") ||
		msg.includes("timed out")
	);
});

function handleRetry() {
	// Find last user message and resend
	for (let i = pane.messages.length - 1; i >= 0; i--) {
		if (pane.messages[i].role === "user") {
			pane.sendMessage(pane.messages[i].message);
			return;
		}
	}
}

function handleChatSend(text: string) {
	pane.sendMessage(text);
}

function handleOpenChange(isOpen: boolean) {
	if (!isOpen) {
		pane.abort();
		onClose();
	}
}
</script>

{#key sheetSide}
	<Sheet.Root {open} onOpenChange={handleOpenChange}>
		<Sheet.Content
			side={sheetSide}
			class={isDesktop
				? "flex h-full w-full flex-col sm:max-w-[28rem]"
				: "flex h-[85dvh] flex-col"}
		>
			<Sheet.Header class="flex-shrink-0 border-b border-[var(--surface-border)]">
				{#if phase === "ask" && scopeArticleId}
					<Sheet.Title class="text-xs font-medium text-[var(--text-secondary)]">
						質問の対象
					</Sheet.Title>
					<div class="flex items-start gap-2.5 rounded-lg border border-[var(--surface-border)] bg-[var(--surface-hover)] p-2.5">
						<FileText class="mt-0.5 h-4 w-4 flex-shrink-0 text-[var(--interactive-text)]" />
						<div class="min-w-0 flex-1">
							<p class="line-clamp-2 text-sm font-medium text-[var(--text-primary)]">{scopeTitle}</p>
							{#if scopeTags && scopeTags.length > 0}
								<div class="mt-1.5 flex flex-wrap gap-1">
									{#each scopeTags as tag}
										<span class="rounded-full bg-[var(--surface-bg)] px-2 py-0.5 text-[10px] text-[var(--text-secondary)]">
											{tag}
										</span>
									{/each}
								</div>
							{/if}
						</div>
					</div>
				{:else if phase === "ask"}
					<Sheet.Title class="text-sm font-semibold text-[var(--text-primary)]">
						{scopeTitle} について質問
					</Sheet.Title>
					{#if scopeContext}
						<Sheet.Description class="text-xs text-[var(--text-secondary)]">
							{scopeContext}
						</Sheet.Description>
					{/if}
				{:else}
					<Sheet.Title class="text-sm font-semibold text-[var(--text-primary)]">
						Ask Augur
					</Sheet.Title>
				{/if}
			</Sheet.Header>

			{#if phase === "ask"}
				<!-- Ask phase: input form with suggestions -->
				<div class="flex flex-1 flex-col gap-3 p-4">
					<input
						type="text"
						bind:value={question}
						placeholder="質問を入力..."
						class="w-full rounded-lg border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2 text-sm text-[var(--text-primary)] focus:border-[var(--accent-primary)] focus:outline-none"
						onkeydown={(e) => {
							if (e.key === "Enter" && !e.isComposing) handleAskSubmit();
						}}
					/>

					<div class="flex items-center gap-2">
						{#key shuffleCount}
							<div class="flex flex-wrap gap-2">
								{#each suggestions as suggestion}
									<button
										type="button"
										class="rounded-full border border-[var(--surface-border)] px-3 py-1 text-xs text-[var(--text-secondary)] transition-colors hover:border-[var(--accent-primary)] hover:text-[var(--accent-primary)]"
										onclick={() => {
											question = suggestion;
										}}
									>
										{suggestion}
									</button>
								{/each}
							</div>
						{/key}
						<button
							type="button"
							class="flex-shrink-0 rounded-full p-1.5 text-[var(--text-secondary)] transition-colors hover:bg-[var(--surface-hover)] hover:text-[var(--accent-primary)]"
							onclick={() => shuffleCount++}
							title="Shuffle suggestions"
						>
							<Shuffle class="h-3.5 w-3.5" />
						</button>
					</div>

					<div class="rounded-lg border border-[var(--surface-border)] bg-[var(--surface-hover)] p-3 text-xs text-[var(--text-secondary)]">
						現在の Knowledge Home のコンテキストを添えて Augur に質問します。
					</div>

					<div class="mt-auto flex justify-end">
						<button
							type="button"
							class="rounded-lg bg-[var(--interactive-text)] px-3 py-2 text-sm font-medium text-white"
							onclick={handleAskSubmit}
						>
							Augur に質問
						</button>
					</div>
				</div>
			{:else}
				<!-- Chat phase: message list + input -->
				<div
					bind:this={chatContainer}
					class="flex-1 overflow-y-auto p-4"
				>
					{#each pane.messages as msg (msg.id)}
						<ChatMessage
							message={msg.message}
							role={msg.role}
							timestamp={msg.timestamp}
							citations={msg.citations}
						/>
					{/each}

					{#if pane.isLoading && pane.messages.at(-1)?.message === ""}
						<div class="mb-4 flex gap-3">
							<div class="relative mt-1 h-8 w-8 flex-shrink-0 overflow-hidden rounded-full border border-border/50 bg-muted shadow-sm">
								<img src={augurAvatar} alt="Augur" class="h-full w-full object-cover" />
								<div class="absolute inset-0 flex items-center justify-center bg-background/40">
									<Loader2 class="h-4 w-4 animate-spin text-primary" />
								</div>
							</div>
							<div class="rounded-2xl rounded-bl-none border border-border/50 bg-muted/50 p-3 text-sm shadow-sm">
								<p class="text-muted-foreground">
									{#if pane.progressStage === "searching"}
										Searching knowledge base...
									{:else if pane.progressStage === "generating"}
										Generating answer...
									{:else}
										Augur is thinking...
									{/if}
								</p>
							</div>
						</div>
					{/if}

					{#if canRetry}
						<div class="mb-4 flex justify-center">
							<button
								type="button"
								class="flex items-center gap-1.5 rounded-full border border-border/50 px-3 py-1.5 text-xs text-muted-foreground hover:bg-muted/50 hover:text-foreground"
								onclick={handleRetry}
							>
								<RotateCcw class="h-3 w-3" />
								Retry
							</button>
						</div>
					{/if}
				</div>

				<div class="flex-shrink-0">
					<ChatInput onSend={handleChatSend} disabled={pane.isLoading} />
				</div>
			{/if}
		</Sheet.Content>
	</Sheet.Root>
{/key}

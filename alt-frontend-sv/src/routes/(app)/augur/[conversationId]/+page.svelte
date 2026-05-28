<script lang="ts">
import { onMount } from "svelte";
import { page } from "$app/stores";
import AugurChat from "$lib/components/desktop/augur/AugurChat.svelte";
import ChatWindow from "$lib/components/mobile/search/ChatWindow.svelte";
import {
	type AugurStoredConversation,
	createClientTransport,
	getAugurConversation,
} from "$lib/connect";
import { useViewport } from "$lib/stores/viewport.svelte";

type CitationKindName = "UNSPECIFIED" | "WEB" | "ARTICLE" | "SUMMARY";

type PaneCitation = {
	URL: string;
	Title: string;
	PublishedAt?: string;
	Kind?: CitationKindName;
	RefID?: string;
};

type PaneMessage = {
	id: string;
	message: string;
	role: "user" | "assistant";
	timestamp: string;
	citations?: PaneCitation[];
	relatedCitations?: PaneCitation[];
};

type MobileCitation = {
	url: string;
	title: string;
	publishedAt: string;
	kind?: CitationKindName;
	refId?: string;
};

type MobileMessage = {
	role: "user" | "assistant";
	content: string;
	citations?: MobileCitation[];
	relatedCitations?: MobileCitation[];
};

const { isDesktop } = useViewport();

let conversation = $state<AugurStoredConversation | null>(null);
let errorMessage = $state<string>("");
let isLoading = $state(true);

const conversationId = $derived($page.params.conversationId ?? "");

function toPaneCitation(
	c: AugurStoredConversation["messages"][number]["citations"][number],
): PaneCitation {
	return {
		URL: c.url,
		Title: c.title,
		PublishedAt: c.publishedAt,
		Kind: c.kind,
		RefID: c.refId,
	};
}

function toMobileCitation(
	c: AugurStoredConversation["messages"][number]["citations"][number],
): MobileCitation {
	return {
		url: c.url,
		title: c.title,
		publishedAt: c.publishedAt,
		kind: c.kind,
		refId: c.refId,
	};
}

function toPaneMessages(conv: AugurStoredConversation): PaneMessage[] {
	return conv.messages.map((m, index) => ({
		id: `${m.role}-${conv.id}-${index}`,
		message: m.content,
		role: m.role,
		timestamp: m.createdAt ? m.createdAt.toLocaleTimeString() : "",
		citations: m.citations.map(toPaneCitation),
		relatedCitations: m.relatedCitations.map(toPaneCitation),
	}));
}

function toMobileMessages(conv: AugurStoredConversation): MobileMessage[] {
	return conv.messages.map((m) => ({
		role: m.role,
		content: m.content,
		citations: m.citations.map(toMobileCitation),
		relatedCitations: m.relatedCitations.map(toMobileCitation),
	}));
}

onMount(() => {
	void load();
});

async function load() {
	if (!conversationId) return;
	isLoading = true;
	errorMessage = "";
	try {
		conversation = await getAugurConversation(
			createClientTransport(),
			conversationId,
		);
	} catch (err) {
		errorMessage = err instanceof Error ? err.message : "Failed to load";
	} finally {
		isLoading = false;
	}
}
</script>

<svelte:head>
	<title>
		{conversation?.title ? `Augur · ${conversation.title}` : "Ask Augur"}
	</title>
</svelte:head>

{#if isLoading}
	<p class="status">Loading conversation…</p>
{:else if errorMessage}
	<p class="status status-error" role="alert">{errorMessage}</p>
{:else if conversation}
	{#if isDesktop}
		<AugurChat
			initialMessages={toPaneMessages(conversation)}
			initialConversationId={conversation.id}
			title={conversation.title}
		/>
	{:else}
		<div class="augur-mobile-shell">
			<ChatWindow
				initialMessages={toMobileMessages(conversation)}
				initialConversationId={conversation.id}
			/>
		</div>
	{/if}
{/if}

<style>
.status {
	font-family: var(--font-mono);
	font-size: 0.75rem;
	letter-spacing: 0.18em;
	text-transform: uppercase;
	color: var(--text-muted);
	text-align: center;
	padding: 3rem 1rem;
}

.status-error {
	color: #b91c1c;
}

.augur-mobile-shell {
	position: fixed;
	top: 0;
	left: 0;
	right: 0;
	bottom: calc(2.75rem + env(safe-area-inset-bottom, 0px));
	overflow: hidden;
}
</style>

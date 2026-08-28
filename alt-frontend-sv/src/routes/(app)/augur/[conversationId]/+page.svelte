<script lang="ts">
import { page } from "$app/stores";
import AugurChat from "$lib/components/desktop/augur/AugurChat.svelte";
import {
	type AugurStoredConversation,
	createClientTransport,
	getAugurConversation,
} from "$lib/connect";
import { formatAugurConversationLabel } from "$lib/utils/augur-entry";

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

let conversation = $state<AugurStoredConversation | null>(null);
let errorMessage = $state<string>("");
let isLoading = $state(true);

const conversationId = $derived($page.params.conversationId ?? "");
// The stored title is the first user turn verbatim, marker and all.
const conversationLabel = $derived(
	formatAugurConversationLabel(conversation?.title ?? ""),
);

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

// Track which id we have loaded / are loading so param changes re-fetch,
// and an empty param cannot leave the page stuck on isLoading=true.
let loadedId = $state("");
let inflightId = $state("");

$effect(() => {
	const id = conversationId;
	if (!id) {
		isLoading = false;
		conversation = null;
		loadedId = "";
		inflightId = "";
		return;
	}
	if (id === loadedId || id === inflightId) return;
	void load(id);
});

async function load(id: string) {
	inflightId = id;
	isLoading = true;
	errorMessage = "";
	try {
		conversation = await getAugurConversation(createClientTransport(), id);
		loadedId = id;
	} catch (err) {
		errorMessage = err instanceof Error ? err.message : "Failed to load";
		loadedId = "";
	} finally {
		if (inflightId === id) inflightId = "";
		isLoading = false;
	}
}
</script>

<svelte:head>
	<title>
		{conversationLabel ? `Augur · ${conversationLabel}` : "Ask Augur"}
	</title>
</svelte:head>

{#if isLoading}
	<p class="status">Loading conversation…</p>
{:else if errorMessage}
	<p class="status status-error" role="alert">{errorMessage}</p>
{:else if conversation}
	<!--
		One chat for every width — see the note in ../+page.svelte. Rotating a
		phone used to swap in a second implementation, which dropped the
		conversation that had just been loaded from the server and any follow-up
		still streaming.
	-->
	<div class="augur-frame">
		<AugurChat
			initialMessages={toPaneMessages(conversation)}
			initialConversationId={conversation.id}
		/>
	</div>
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

.augur-frame {
	position: fixed;
	top: 0;
	left: 0;
	right: 0;
	bottom: calc(2.75rem + env(safe-area-inset-bottom, 0px));
	overflow: hidden;
}

/* The same 48rem the `md:` utilities and `isDesktop()` use. */
@media (min-width: 48rem) {
	.augur-frame {
		position: static;
		overflow: visible;
	}
}
</style>

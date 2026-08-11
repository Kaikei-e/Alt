interface ResolveAugurEntryInput {
	q?: string | null;
	context?: string | null;
	articleId?: string | null;
}

export function buildAugurInitialMessage(
	question: string,
	context?: string | null,
	articleId?: string | null,
): string {
	const trimmedQuestion = question.trim();
	const trimmedContext = context?.trim() ?? "";

	if (!trimmedContext) {
		return trimmedQuestion;
	}

	if (articleId) {
		return `Regarding the article: ${trimmedContext} [articleId: ${articleId}]\n\nQuestion:\n${trimmedQuestion}`;
	}

	return `Context:\n${trimmedContext}\n\nQuestion:\n${trimmedQuestion}`;
}

const ARTICLE_PREFIX = "Regarding the article: ";
const QUESTION_SEPARATOR = "\n\nQuestion:\n";
const ARTICLE_ID_PREFIX = "[articleId: ";
const UUID_PATTERN =
	/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export interface AugurUserMessageView {
	/** The question as the reader wrote it. */
	question: string;
	/** The article the question was asked about, when there is one. */
	articleTitle: string | null;
	/**
	 * Set only when the id is one rag-orchestrator would itself accept as a
	 * scope, so a thumbnail is never requested for an id the backend refused.
	 */
	articleId: string | null;
}

/**
 * Splits a stored Augur turn into the parts a person should see.
 *
 * The article id travels inside the message text because that is the contract
 * rag-orchestrator parses to scope retrieval (`ParseQueryIntent` in
 * query_intent.go), so the wire format cannot drop it. The steps below mirror
 * that parse — prefix, last separator, marker from the end — precisely so that
 * what is hidden here is exactly what the backend consumed there. A title with
 * brackets in it, which publishers produce constantly, survives both.
 */
export function parseAugurUserMessage(message: string): AugurUserMessageView {
	const plain: AugurUserMessageView = {
		question: message,
		articleTitle: null,
		articleId: null,
	};

	if (!message.startsWith(ARTICLE_PREFIX)) return plain;

	const separatorIndex = message.lastIndexOf(QUESTION_SEPARATOR);
	if (separatorIndex < 0) return plain;

	const header = message.slice(ARTICLE_PREFIX.length, separatorIndex);
	const question = message
		.slice(separatorIndex + QUESTION_SEPARATOR.length)
		.trim();

	const markerStart = header.lastIndexOf(ARTICLE_ID_PREFIX);
	if (markerStart < 0) {
		return { question, articleTitle: header.trim(), articleId: null };
	}

	const markerEnd = header.indexOf("]", markerStart);
	if (markerEnd < 0) {
		return { question, articleTitle: header.trim(), articleId: null };
	}

	const rawId = header
		.slice(markerStart + ARTICLE_ID_PREFIX.length, markerEnd)
		.trim();

	return {
		question,
		articleTitle: header.slice(0, markerStart).trim(),
		articleId: UUID_PATTERN.test(rawId) ? rawId : null,
	};
}

const ARTICLE_ID_MARKER = /\s*\[articleId:[^\]]*\]/gi;
const ENVELOPE_PREFIX = /^(?:Regarding the article:|Context:)\s*/;

/**
 * Cleans a stored conversation title or message preview for display.
 *
 * rag-orchestrator names a conversation after its first user turn, verbatim and
 * whitespace-collapsed (`titleFromFirstMessage`), so every article-scoped chat
 * already carries the marker in a field meant for people. Cleaning it here
 * rather than only at the write covers the rows that exist, and the collapsed
 * form the structured parse above cannot see.
 */
export function formatAugurConversationLabel(raw: string): string {
	const collapsed = raw
		.replace(ARTICLE_ID_MARKER, "")
		.replace(/\s+/g, " ")
		.trim();
	if (!collapsed || !ENVELOPE_PREFIX.test(collapsed)) return collapsed;

	return collapsed
		.replace(ENVELOPE_PREFIX, "")
		.replace(/\s*Question:\s*/, " — ")
		.trim();
}

export function resolveAugurEntry({
	q,
	context,
	articleId,
}: ResolveAugurEntryInput): {
	initialDraft: string;
	initialMessage: string;
} {
	const trimmedQuestion = q?.trim() ?? "";
	const trimmedContext = context?.trim() ?? "";

	if (trimmedQuestion) {
		return {
			initialDraft: "",
			initialMessage: buildAugurInitialMessage(
				trimmedQuestion,
				trimmedContext,
				articleId,
			),
		};
	}

	return {
		initialDraft: trimmedContext,
		initialMessage: "",
	};
}

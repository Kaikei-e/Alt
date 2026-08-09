const GENERIC_FALLBACK_MESSAGE =
	"I couldn't find enough information to answer that properly. Please try a more specific question.";
const INSUFFICIENT_CONTEXT_MESSAGE =
	"Not enough indexed evidence was available yet. Please try a more specific question.";
const ARTICLE_NOT_INDEXED_MESSAGE =
	"This article hasn't been indexed yet, so there's nothing to answer from. It should become available once indexing catches up.";
const CAUSAL_FALLBACK_MESSAGE =
	"I couldn't establish a consistent enough evidence trail to explain the cause confidently. Please try a more specific question.";
const SERVICE_FAILURE_MESSAGE =
	"Something went wrong on our side while answering. Nothing is wrong with your question — please try again.";

/**
 * Maps a server fallback code to what the reader is told.
 *
 * The codes are the stable contract (rag-orchestrator's fallback_code.go); the
 * sentences are ours. What matters here is who the message blames. Every
 * infrastructure failure used to render as "try a more specific question",
 * which is advice the reader cannot act on when the LLM is down — and for an
 * unindexed article, no rephrasing can ever succeed.
 *
 * Unknown codes fall through to the generic message on purpose: the server adds
 * codes over time, and an older client must not treat one as an error.
 */
export function formatAugurFallbackMessage(code: string): string {
	const trimmed = code.trim();
	if (!trimmed) {
		return GENERIC_FALLBACK_MESSAGE;
	}

	// Codes are matched as a prefix so a server that later appends detail
	// ("LLM_UNAVAILABLE: dial tcp …") still resolves to the right message.
	const upper = trimmed.toUpperCase();
	const is = (prefix: string) => upper.startsWith(prefix);

	if (is("ARTICLE_NOT_INDEXED")) {
		return ARTICLE_NOT_INDEXED_MESSAGE;
	}

	if (
		is("LLM_UNAVAILABLE") ||
		is("LLM_STREAM_FAILED") ||
		is("LLM_NO_OUTPUT") ||
		is("VALIDATION_FAILED") ||
		is("GENERATION_FAILED") ||
		is("RETRIEVAL_FAILED")
	) {
		return SERVICE_FAILURE_MESSAGE;
	}

	if (is("CAUSAL_TRAIL_INSUFFICIENT")) {
		return CAUSAL_FALLBACK_MESSAGE;
	}

	if (
		is("RETRIEVAL_EMPTY") ||
		is("RELEVANCE_INSUFFICIENT") ||
		is("ANSWER_DECLINED") ||
		is("ANSWER_REJECTED")
	) {
		return INSUFFICIENT_CONTEXT_MESSAGE;
	}

	// Pre-code servers sent the raw error string. Keep the one signal that was
	// reliable in that format so a rolling deploy does not regress.
	if (upper.includes("INSUFFICIENT")) {
		return INSUFFICIENT_CONTEXT_MESSAGE;
	}

	return GENERIC_FALLBACK_MESSAGE;
}

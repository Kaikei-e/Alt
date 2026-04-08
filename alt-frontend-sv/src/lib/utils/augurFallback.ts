const GENERIC_FALLBACK_MESSAGE =
	"I couldn't find enough information to answer that properly. Please try a more specific question.";
const INSUFFICIENT_CONTEXT_MESSAGE =
	"Not enough indexed evidence was available yet. Please try a more specific question.";
const CAUSAL_FALLBACK_MESSAGE =
	"I couldn't establish a consistent enough evidence trail to explain the cause confidently. Please try a more specific question.";
const japanesePattern = /[\u3040-\u30ff\u3400-\u9fff]/;

export function formatAugurFallbackMessage(code: string): string {
	const trimmed = code.trim();
	if (!trimmed) {
		return GENERIC_FALLBACK_MESSAGE;
	}

	const lower = trimmed.toLowerCase();
	if (
		lower.includes("insufficient") ||
		lower.includes("retrieval quality insufficient")
	) {
		return INSUFFICIENT_CONTEXT_MESSAGE;
	}

	if (japanesePattern.test(trimmed)) {
		return CAUSAL_FALLBACK_MESSAGE;
	}

	return GENERIC_FALLBACK_MESSAGE;
}

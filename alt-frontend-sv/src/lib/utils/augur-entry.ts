interface ResolveAugurEntryInput {
	q?: string | null;
	context?: string | null;
}

export function buildAugurInitialMessage(
	question: string,
	context?: string | null,
): string {
	const trimmedQuestion = question.trim();
	const trimmedContext = context?.trim() ?? "";

	if (!trimmedContext) {
		return trimmedQuestion;
	}

	return `Context:\n${trimmedContext}\n\nQuestion:\n${trimmedQuestion}`;
}

export function resolveAugurEntry({
	q,
	context,
}: ResolveAugurEntryInput): {
	initialDraft: string;
	initialMessage: string;
} {
	const trimmedQuestion = q?.trim() ?? "";
	const trimmedContext = context?.trim() ?? "";

	if (trimmedQuestion) {
		return {
			initialDraft: "",
			initialMessage: buildAugurInitialMessage(trimmedQuestion, trimmedContext),
		};
	}

	return {
		initialDraft: trimmedContext,
		initialMessage: "",
	};
}

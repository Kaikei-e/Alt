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

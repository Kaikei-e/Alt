import { describe, expect, it } from "vitest";
import {
	buildAugurInitialMessage,
	formatAugurConversationLabel,
	parseAugurUserMessage,
	resolveAugurEntry,
} from "./augur-entry";

describe("augur-entry", () => {
	it("builds a question-only message when no context is provided", () => {
		expect(buildAugurInitialMessage("What changed today?")).toBe(
			"What changed today?",
		);
	});

	it("builds a scoped question when context is provided", () => {
		expect(
			buildAugurInitialMessage(
				"What is new here?",
				"Article summary about AI chips",
			),
		).toBe(
			"Context:\nArticle summary about AI chips\n\nQuestion:\nWhat is new here?",
		);
	});

	it("builds a structured reference when articleId is provided", () => {
		expect(
			buildAugurInitialMessage(
				"What is the key point?",
				"Apple Announces M5 Chip",
				"abc123",
			),
		).toBe(
			"Regarding the article: Apple Announces M5 Chip [articleId: abc123]\n\nQuestion:\nWhat is the key point?",
		);
	});

	it("uses context format when articleId is absent even with context", () => {
		expect(buildAugurInitialMessage("Explain this", "Some context text")).toBe(
			"Context:\nSome context text\n\nQuestion:\nExplain this",
		);
	});

	it("uses q for auto-send and keeps context-only as a draft", () => {
		expect(
			resolveAugurEntry({
				q: "Explain this",
				context: "Short summary",
			}),
		).toEqual({
			initialDraft: "",
			initialMessage: "Context:\nShort summary\n\nQuestion:\nExplain this",
		});

		expect(
			resolveAugurEntry({
				q: "",
				context: "Short summary",
			}),
		).toEqual({
			initialDraft: "Short summary",
			initialMessage: "",
		});
	});

	it("passes articleId through to initialMessage in resolveAugurEntry", () => {
		expect(
			resolveAugurEntry({
				q: "What changed?",
				context: "AI Chip Breakthrough",
				articleId: "xyz789",
			}),
		).toEqual({
			initialDraft: "",
			initialMessage:
				"Regarding the article: AI Chip Breakthrough [articleId: xyz789]\n\nQuestion:\nWhat changed?",
		});
	});

	it("resolveAugurEntry without articleId preserves existing behavior", () => {
		expect(
			resolveAugurEntry({
				q: "What changed?",
				context: "AI Chip Breakthrough",
			}),
		).toEqual({
			initialDraft: "",
			initialMessage:
				"Context:\nAI Chip Breakthrough\n\nQuestion:\nWhat changed?",
		});
	});
});

describe("parseAugurUserMessage", () => {
	const ARTICLE_ID = "c6463b44-589f-4959-879f-40eda019f95a";

	it("keeps a plain question exactly as the reader wrote it", () => {
		expect(parseAugurUserMessage("What changed today?")).toEqual({
			question: "What changed today?",
			articleTitle: null,
			articleId: null,
		});
	});

	it("splits an article-scoped message into title and question", () => {
		const message = buildAugurInitialMessage(
			"3行でまとめると？",
			"Love me, love my bruised ego | Film | The Guardian",
			ARTICLE_ID,
		);

		expect(parseAugurUserMessage(message)).toEqual({
			question: "3行でまとめると？",
			articleTitle: "Love me, love my bruised ego | Film | The Guardian",
			articleId: ARTICLE_ID,
		});
	});

	it("never leaves the article id anywhere in the reader-facing parts", () => {
		const parsed = parseAugurUserMessage(
			buildAugurInitialMessage("要点は？", "Some Article", ARTICLE_ID),
		);

		expect(parsed.question).not.toContain(ARTICLE_ID);
		expect(parsed.articleTitle).not.toContain(ARTICLE_ID);
		expect(parsed.articleTitle).not.toContain("articleId");
	});

	// The title is publisher-controlled and routinely carries brackets, so the
	// marker is found from the end — the same step-based parse rag-orchestrator
	// applies to the very same string (query_intent.go).
	it("keeps brackets that belong to the title", () => {
		const parsed = parseAugurUserMessage(
			`Regarding the article: [Breaking] New AI Model [v2.0] Released [articleId: ${ARTICLE_ID}]\n\nQuestion:\nWhat changed?`,
		);

		expect(parsed.articleTitle).toBe("[Breaking] New AI Model [v2.0] Released");
		expect(parsed.articleId).toBe(ARTICLE_ID);
	});

	it("splits at the last separator so a quoted Question: block stays in the body", () => {
		const parsed = parseAugurUserMessage(
			`Regarding the article: Title [articleId: ${ARTICLE_ID}]\n\nQuestion:\nWhy does\n\nQuestion:\nappear twice?`,
		);

		expect(parsed.question).toBe("appear twice?");
	});

	// rag-orchestrator only treats a marker as an article scope when it parses
	// as a UUID. Mirroring that keeps a thumbnail from being requested for an
	// id the backend itself refused to scope on.
	it("drops a non-UUID marker from the display without claiming a scope", () => {
		const parsed = parseAugurUserMessage(
			"Regarding the article: Some Article [articleId: abc123]\n\nQuestion:\nWhat is this?",
		);

		expect(parsed).toEqual({
			question: "What is this?",
			articleTitle: "Some Article",
			articleId: null,
		});
	});

	it("returns the message untouched when the envelope is incomplete", () => {
		const message = `Regarding the article: Title [articleId: ${ARTICLE_ID}] but no question separator`;

		expect(parseAugurUserMessage(message)).toEqual({
			question: message,
			articleTitle: null,
			articleId: null,
		});
	});

	it("leaves the free-form context envelope alone", () => {
		const message = "Context:\nKnowledge Home\n\nQuestion:\nWhat is new?";

		expect(parseAugurUserMessage(message)).toEqual({
			question: message,
			articleTitle: null,
			articleId: null,
		});
	});
});

// rag-orchestrator stores the first user turn verbatim as the conversation
// title, whitespace-collapsed (titleFromFirstMessage). Existing rows therefore
// carry the marker, so the label is cleaned where it is displayed rather than
// only where it is written.
describe("formatAugurConversationLabel", () => {
	const ARTICLE_ID = "c6463b44-589f-4959-879f-40eda019f95a";

	it("turns a stored article-scoped title into article and question", () => {
		const stored = `Regarding the article: Love me, love my bruised ego | The Guardian [articleId: ${ARTICLE_ID}] Question: 3行でまとめると？`;

		expect(formatAugurConversationLabel(stored)).toBe(
			"Love me, love my bruised ego | The Guardian — 3行でまとめると？",
		);
	});

	it("handles the un-collapsed message the same way", () => {
		const raw = buildAugurInitialMessage(
			"3行でまとめると？",
			"Love me, love my bruised ego | The Guardian",
			ARTICLE_ID,
		);

		expect(formatAugurConversationLabel(raw)).toBe(
			"Love me, love my bruised ego | The Guardian — 3行でまとめると？",
		);
	});

	it("strips a truncated title's marker without leaving a bare bracket", () => {
		const stored = `Regarding the article: Some Article [articleId: ${ARTICLE_ID}] Question: 要点…`;

		const label = formatAugurConversationLabel(stored);
		expect(label).not.toContain(ARTICLE_ID);
		expect(label).not.toContain("[");
	});

	it("unwraps the free-form context envelope too", () => {
		expect(
			formatAugurConversationLabel(
				"Context:\nKnowledge Home\n\nQuestion:\nWhat is new?",
			),
		).toBe("Knowledge Home — What is new?");
	});

	it("leaves an ordinary title alone", () => {
		expect(formatAugurConversationLabel("What changed today?")).toBe(
			"What changed today?",
		);
	});

	it("returns an empty string for an empty title", () => {
		expect(formatAugurConversationLabel("   ")).toBe("");
	});
});

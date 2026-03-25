import { describe, expect, it } from "vitest";
import { buildAugurInitialMessage, resolveAugurEntry } from "./augur-entry";

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

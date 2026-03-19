import { describe, expect, it } from "vitest";
import {
	buildAugurInitialMessage,
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

	it("uses q for auto-send and keeps context-only as a draft", () => {
		expect(
			resolveAugurEntry({
				q: "Explain this",
				context: "Short summary",
			}),
		).toEqual({
			initialDraft: "",
			initialMessage:
				"Context:\nShort summary\n\nQuestion:\nExplain this",
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
});

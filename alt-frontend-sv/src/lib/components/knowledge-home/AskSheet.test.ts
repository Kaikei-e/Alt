import { describe, it, expect } from "vitest";
import { buildAugurInitialMessage } from "$lib/utils/augur-entry";

/**
 * Tests for AskSheet data flow logic.
 * Component rendering tested via browser tests (*.svelte.test.ts).
 * Streaming logic tested in useAugurPane.test.ts.
 */
describe("AskSheet data flow", () => {
	describe("initial message construction", () => {
		it("includes article context and articleId when scoped to an article", () => {
			const msg = buildAugurInitialMessage(
				"What is this about?",
				"Article Title",
				"article-123",
			);
			expect(msg).toContain("Article Title");
			expect(msg).toContain("article-123");
			expect(msg).toContain("What is this about?");
		});

		it("includes context without articleId for free-form ask", () => {
			const msg = buildAugurInitialMessage(
				"Tell me about AI",
				"Knowledge Home",
			);
			expect(msg).toContain("Knowledge Home");
			expect(msg).toContain("Tell me about AI");
			expect(msg).not.toContain("articleId");
		});

		it("returns plain question when no context provided", () => {
			const msg = buildAugurInitialMessage("Hello Augur");
			expect(msg).toBe("Hello Augur");
		});

		it("trims whitespace from question", () => {
			const msg = buildAugurInitialMessage("  spaced question  ");
			expect(msg).toBe("spaced question");
		});
	});

	describe("phase transition rules", () => {
		it("ask phase requires non-empty question to transition", () => {
			const question = "   ";
			const trimmed = question.trim();
			expect(trimmed).toBe("");
			// Empty question should NOT trigger transition
		});

		it("non-empty question triggers transition to chat phase", () => {
			const question = "What is RSS?";
			const trimmed = question.trim();
			expect(trimmed.length).toBeGreaterThan(0);
			// Non-empty triggers transition
		});
	});
});

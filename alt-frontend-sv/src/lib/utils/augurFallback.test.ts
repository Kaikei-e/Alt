import { describe, it, expect } from "vitest";

import { formatAugurFallbackMessage } from "./augurFallback";

const ADVICE_TO_REPHRASE = "more specific question";

describe("formatAugurFallbackMessage", () => {
	it("does not blame the question when the failure is ours", () => {
		// A dead LLM, a broken stream and a failed search are all our problem.
		// Telling the reader to ask a more specific question is advice they
		// cannot act on, and it hides an outage behind what reads as a quality
		// limitation.
		for (const code of [
			"LLM_UNAVAILABLE",
			"LLM_STREAM_FAILED",
			"LLM_NO_OUTPUT",
			"VALIDATION_FAILED",
			"GENERATION_FAILED",
			"RETRIEVAL_FAILED",
		]) {
			expect(formatAugurFallbackMessage(code)).not.toContain(
				ADVICE_TO_REPHRASE,
			);
		}
	});

	it("says an unindexed article is unindexed", () => {
		// This is the "ask about this article" entry point. No rephrasing can
		// ever succeed, so suggesting one sends the reader in circles.
		const message = formatAugurFallbackMessage("ARTICLE_NOT_INDEXED");
		expect(message).not.toContain(ADVICE_TO_REPHRASE);
		expect(message.toLowerCase()).toContain("index");
	});

	it("still suggests rephrasing when the retrieval genuinely came up short", () => {
		for (const code of [
			"RETRIEVAL_EMPTY",
			"RELEVANCE_INSUFFICIENT",
			"ANSWER_DECLINED",
			"ANSWER_REJECTED",
		]) {
			expect(formatAugurFallbackMessage(code)).toContain(ADVICE_TO_REPHRASE);
		}
	});

	it("falls back to a generic message for unknown or empty codes", () => {
		// Codes are additive on the server, and an older client must not treat an
		// unrecognised one as an error.
		expect(formatAugurFallbackMessage("")).toBeTruthy();
		expect(formatAugurFallbackMessage("SOME_FUTURE_CODE")).toBeTruthy();
		expect(formatAugurFallbackMessage("   ")).toBeTruthy();
	});

	it("does not decide the message from whether the text contains Japanese", () => {
		// The previous implementation switched on a CJK character test, so any
		// Japanese reason string became a causal-explanation notice regardless of
		// what actually failed.
		const japaneseButInfrastructural = formatAugurFallbackMessage(
			"LLM_UNAVAILABLE: 接続できませんでした",
		);
		expect(japaneseButInfrastructural).not.toContain(ADVICE_TO_REPHRASE);
	});
});

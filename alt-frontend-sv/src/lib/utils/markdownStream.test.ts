/**
 * splitSettledBlocks — the render-path half of the typewriter restoration.
 *
 * A streaming answer re-parsed and re-rendered from scratch on every frame is
 * what made the old typewriter flicker. Splitting the text at the last block
 * boundary lets ThreadEntry hand Svelte an unchanged `settled` string (same
 * value ⇒ `{@html}` leaves the DOM alone) and re-render only the tail block
 * that is still being written.
 */
import { describe, expect, it } from "vitest";
import { splitSettledBlocks } from "./markdownStream";

describe("splitSettledBlocks", () => {
	it("splits at the last blank line so completed blocks stop being re-parsed", () => {
		const text =
			"First paragraph.\n\nSecond paragraph.\n\nThird still being wr";

		const { settled, tail } = splitSettledBlocks(text);

		expect(settled).toBe("First paragraph.\n\nSecond paragraph.\n\n");
		expect(tail).toBe("Third still being wr");
	});

	it("puts everything in the tail when no blank line has arrived yet", () => {
		const text = "The very first paragraph, still stre";

		const { settled, tail } = splitSettledBlocks(text);

		expect(settled).toBe("");
		expect(tail).toBe(text);
	});

	it("keeps an unclosed code fence whole", () => {
		// Splitting inside an open fence would render two <pre> blocks where
		// the finished answer has one.
		const text = "Intro.\n\n```ts\nconst a = 1;\n\nconst b = 2";

		const { settled, tail } = splitSettledBlocks(text);

		expect(settled).toBe("");
		expect(tail).toBe(text);
	});

	it("splits again once the fence has closed", () => {
		const text = "Intro.\n\n```ts\nconst a = 1;\n```\n\nAfter the fence, str";

		const { settled, tail } = splitSettledBlocks(text);

		expect(settled).toBe("Intro.\n\n```ts\nconst a = 1;\n```\n\n");
		expect(tail).toBe("After the fence, str");
	});

	it("always reconstitutes the input from settled + tail", () => {
		// Deterministic pseudo-random corpus: fragments that exercise blank
		// lines, fences, headings, lists, CJK and emoji in varied orders.
		const fragments = [
			"Paragraph text. ",
			"\n",
			"\n\n",
			"# Heading\n",
			"- list item\n",
			"```",
			"```\n",
			"code line\n",
			"**bold** and *italic* ",
			"日本語の本文。",
			"emoji 🎉👍 ",
			"> quote\n",
			"1. ordered\n",
			"---\n",
		];
		let seed = 0xc0ffee;
		const nextRandom = () => {
			// xorshift32: deterministic across runs, no RNG flake
			seed ^= seed << 13;
			seed ^= seed >>> 17;
			seed ^= seed << 5;
			return (seed >>> 0) / 0xffffffff;
		};

		for (let caseIndex = 0; caseIndex < 50; caseIndex++) {
			const parts = Math.floor(nextRandom() * 12);
			let text = "";
			for (let i = 0; i < parts; i++) {
				text += fragments[Math.floor(nextRandom() * fragments.length)];
			}

			const { settled, tail } = splitSettledBlocks(text);

			expect(settled + tail).toBe(text);
		}
	});
});

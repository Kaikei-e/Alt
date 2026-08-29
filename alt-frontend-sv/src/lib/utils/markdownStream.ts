/**
 * Split a still-streaming answer into blocks that will not change again and
 * the block currently being written. `settled + tail === text`, always.
 *
 * Rendered as two adjacent `{@html parseMarkdown(...)}` regions, the settled
 * half keeps the same string value from frame to frame, so Svelte leaves its
 * DOM subtree alone; only the tail block is re-parsed as the typewriter
 * reveals it. Without this split, every revealed character tears down and
 * rebuilds the whole prose subtree — the flicker ADR-000985 records.
 *
 * Known artefact, accepted: while a code fence is open the whole message
 * stays in the tail (an unclosed fence split across the boundary would render
 * as two <pre> blocks), so a very long code block streams without the
 * settled-prefix optimization until its closing fence arrives.
 */
export function splitSettledBlocks(text: string): {
	settled: string;
	tail: string;
} {
	const i = text.lastIndexOf("\n\n");
	if (i === -1) return { settled: "", tail: text };
	const settled = text.slice(0, i + 2);
	// An unclosed code fence must not be split across two <pre> blocks.
	if ((settled.match(/^```/gm) ?? []).length % 2 === 1) {
		return { settled: "", tail: text };
	}
	return { settled, tail: text.slice(i + 2) };
}

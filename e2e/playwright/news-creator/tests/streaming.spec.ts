import { test, expect, chatBody, summarizeBody } from "../src/fixtures.js";
import { expectHeader, expectHeaderContains, expectStatus } from "../../_shared/http.js";
import { chatStreamChunkSchema } from "../src/schemas.js";

/**
 * The two streaming surfaces — the port of `10-summarize-streaming.hurl` and
 * `11-chat-streaming.hurl`.
 *
 * Both Hurl scenarios could only assert `body contains "..."` on the
 * assembled body, because Hurl reads a chunked response to completion before
 * asserting. Substring checks on a stream are weak in a specific way: `body
 * contains "\"done\": true"` passes for a body that is that literal text
 * inside an error message, and `body matches "data: \".*\"\n\n"` passes for a
 * single well-formed frame followed by garbage. Here the framing is parsed,
 * so a malformed frame anywhere in the stream fails.
 */

/** Splits an SSE body into its frames, dropping the trailing empty segment. */
function sseFrames(body: string): string[] {
	return body.split("\n\n").filter((frame) => frame.length > 0);
}

test.describe("summarize streaming (SSE)", () => {
	test("frames every token as an SSE data event @slow @contract", async ({ api, seed }) => {
		const response = await api.post("/api/v1/summarize", {
			data: summarizeBody(seed.articleId, { stream: true }),
		});
		await expectStatus(response, 200);

		// The three headers the alt-frontend-sv reading view depends on.
		// `X-Accel-Buffering: no` is the load-bearing one: without it nginx
		// buffers the whole response and the "incremental rendering" the SSE
		// path exists for silently becomes a single late blob — a regression
		// that is invisible in any test that only reads the assembled body.
		expectHeaderContains(response, "Content-Type", "text/event-stream");
		expectHeaderContains(response, "Cache-Control", "no-cache");
		expectHeader(response, "X-Accel-Buffering", "no");

		const frames = sseFrames(await response.text());
		expect(frames.length, "the SSE stream carried no frames").toBeGreaterThan(0);

		const tokens: string[] = [];
		for (const frame of frames) {
			// `stream_with_heartbeat` emits exactly three kinds of frame: a data
			// frame, a bare `:` comment heartbeat, and an `event: error` frame on
			// a failed generation (handler/summarize_handler.py). Anything else is
			// a framing bug, and the error frame in particular must fail this test
			// loudly — the handler yields it *inside a 200*, so a stream that
			// died halfway is otherwise indistinguishable from a short summary.
			if (frame.startsWith(":")) continue;
			expect(
				frame.startsWith("event: error"),
				`the stream carried an SSE error event, which the handler emits inside ` +
					`a 200 when generation fails: ${frame.slice(0, 300)}`,
			).toBe(false);
			expect(frame.startsWith("data: "), `unrecognised SSE frame: ${frame.slice(0, 200)}`).toBe(
				true,
			);

			// Each payload is `json.dumps(chunk)` of a *string*. Parsing rather
			// than substring-matching is what catches a chunk containing a raw
			// newline or an unescaped quote — which would split one token into two
			// frames on the client and corrupt the rendered summary.
			const payload: unknown = JSON.parse(frame.slice("data: ".length));
			expect(typeof payload, `SSE data payload was not a JSON string: ${frame.slice(0, 200)}`).toBe(
				"string",
			);
			tokens.push(payload as string);
		}

		// The stub emits three distinct ASCII tokens; the first and last prove
		// the stream was not truncated at either end.
		const assembled = tokens.join("");
		expect(assembled).toContain("stub-token-alpha");
		expect(assembled).toContain("stub-token-gamma");
	});
});

test.describe("chat streaming (NDJSON)", () => {
	test("emits one JSON object per line, terminated by done @contract", async ({ api, seed }) => {
		const response = await api.post("/api/chat", {
			data: chatBody(`stub chat streaming smoke test ${seed.token}`, true),
		});
		await expectStatus(response, 200);

		// NDJSON, not SSE — the two streaming paths in this service use different
		// framing on purpose (Ask Augur consumes Ollama's native NDJSON), and a
		// handler that answered `text/event-stream` here would break the client
		// while still passing every body assertion.
		expectHeaderContains(response, "Content-Type", "application/x-ndjson");
		expectHeaderContains(response, "Cache-Control", "no-cache");
		expectHeader(response, "X-Accel-Buffering", "no");

		const lines = (await response.text()).split("\n").filter((line) => line.trim() !== "");
		expect(lines.length, "the NDJSON stream carried no chunks").toBeGreaterThan(0);

		const contents: string[] = [];
		let doneCount = 0;
		for (const line of lines) {
			// `ndjson_generator` re-serializes each upstream chunk with
			// `json.dumps(chunk) + "\n"` (chat_handler.py:107). The Hurl scenario
			// asserted the literal `"done": true` — json.dumps' default
			// space-after-colon separator — which pins a serializer detail rather
			// than the contract. Parsing each line asserts the thing clients
			// actually depend on: every line is a complete JSON object.
			const chunk = chatStreamChunkSchema.parse(JSON.parse(line));
			contents.push(chunk.message.content);
			if (chunk.done) doneCount += 1;
		}

		// A stream that never sets `done` leaves Ask Augur's reader waiting for a
		// terminator that will not come — a hang, not an error, so nothing
		// downstream reports it. Exactly one, and it has to be the last line: a
		// `done: true` in the middle would make a conforming client stop reading
		// and silently truncate the answer.
		expect(doneCount, "expected exactly one NDJSON chunk with done: true").toBe(1);
		const lastLine = lines.at(-1);
		expect(lastLine, "the NDJSON stream had no final line").toBeDefined();
		expect(chatStreamChunkSchema.parse(JSON.parse(lastLine ?? "")).done).toBe(true);

		const assembled = contents.join("");
		expect(assembled).toContain("stub-token-alpha");
		expect(assembled).toContain("stub-token-gamma");
	});
});

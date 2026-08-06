import {
	KEYWORD_CONTENT,
	KEYWORD_TITLE,
	SMOKE_CONTENT,
	SMOKE_TITLE,
	expect,
	extractTags,
	test,
} from "../src/fixtures.js";

/**
 * `POST /api/v1/extract-tags` — the ports of `02-extract-tags-smoke.hurl`,
 * `03-extract-tags-keyword.hurl` and `07-extract-tags-empty-strings.hurl`,
 * plus the pipeline branches those three never reached.
 *
 * Two things changed in the port beyond adding cases.
 *
 * The retries are gone. Scenario 02 carried `retry: 180` and scenario 03
 * another `retry: 10`, both absorbing the same window — `/health` is 200
 * while `_background_tag_service` is still None. That window is a property of
 * the stack, so it is waited out once in `setup/global-setup.ts` and every
 * test here faces a warm extractor. A 503 reaching one of these tests is now
 * a real finding rather than something to sleep through.
 *
 * The assertions are whole-envelope. `jsonpath "$.tags" isCollection` passes
 * for a handler that has stopped extracting anything; `extractTagsSchema`
 * pins the shape *and* the two invariants that tie the fields to each other
 * (see src/schemas.ts).
 */
test.describe("extract-tags", () => {
	test("a minimal payload returns a well-formed extraction", {
		tag: "@smoke",
	}, async ({ api }) => {
		// Scenario 02's body verbatim. Its assertions were the weak ones —
		// `tags` a collection, `confidence`/`inference_ms` numeric — because
		// there is no guarantee a nine-word sentence yields tags at all. The
		// schema keeps that freedom (`tags` may be empty) while pinning
		// everything that is guaranteed, including that confidence and tags
		// agree with one another.
		const body = await extractTags(api, { title: SMOKE_TITLE, content: SMOKE_CONTENT });
		expect(body.inference_ms).toBeGreaterThan(0);
	});

	test("keyword-dense English text yields tags", { tag: "@contract" }, async ({ api }) => {
		// Scenario 03's body verbatim, and the reason it must stay verbatim:
		// "at least one tag" and "confidence > 0" are only defensible because
		// this exact text produced them in CI. The scenario's *other* job —
		// warming SBERT for the round-trip scenario — is now global setup's.
		const body = await extractTags(api, { title: KEYWORD_TITLE, content: KEYWORD_CONTENT });

		expect(body.tags.length).toBeGreaterThan(0);
		expect(body.confidence).toBeGreaterThan(0);
		expect(body.inference_ms).toBeGreaterThan(0);

		// New: the elements themselves. `_run_extraction` merges a primary
		// KeyBERT pass with a frequency fallback and dedupes the union
		// (tag_extractor/extract.py:258-268), so duplicates in the response
		// would mean that merge regressed — invisible to any per-JSONPath
		// check, and a real bug for a caller that indexes tags by name.
		expect(new Set(body.tags).size, "tags must be deduplicated").toBe(body.tags.length);
		for (const tag of body.tags) {
			expect(tag.trim(), "a blank tag is never a useful tag").not.toBe("");
		}

		// langdetect over this paragraph; the value routes the whole pipeline
		// (English vs Japanese extractor), so it is worth pinning rather than
		// asserting mere presence as the Hurl scenario did.
		expect(body.language).toBe("en");
	});

	test("empty strings degrade gracefully instead of erroring", {
		tag: "@contract",
	}, async ({ api }) => {
		// Scenario 07. Pydantic accepts empty strings — `ExtractTagsRequest`
		// puts no `min_length` on either field — so the request reaches the
		// extractor, where the sanitizer rejects it ("Title too short",
		// input_sanitizer.py:198) and the first early return fires.
		//
		// Strengthened: the Hurl scenario asserted `language exists`, which is
		// true of any string. The value is `"und"` specifically
		// (extract.py:144), and that literal is the only externally visible
		// signal that the pipeline *declined* rather than *ran and found
		// nothing* — a distinction a caller deciding whether to retry needs.
		const body = await extractTags(api, { title: "", content: "" });

		expect(body.tags).toEqual([]);
		expect(body.confidence).toBe(0);
		expect(body.inference_ms).toBe(0);
		expect(body.language).toBe("und");
	});

	test("text below the minimum length declines without running the model", {
		tag: "@contract",
	}, async ({ api }) => {
		// New. A distinct early return from the empty-string one: the sanitizer
		// passes (`min_title_length`/`min_content_length` are both 1), and the
		// *combined* sanitized text then falls under
		// `TagExtractionConfig.min_text_length` = 10, so extract.py:157-167
		// returns before any embedding work.
		//
		// This is the branch a caller hits with a stub or placeholder article,
		// and its contract is that it costs nothing: `inference_ms` of exactly
		// 0 is the assertion that no forward pass happened.
		const body = await extractTags(api, { title: "ab", content: "cd" });

		expect(body.tags).toEqual([]);
		expect(body.language).toBe("und");
		expect(body.inference_ms, "no model may run below min_text_length").toBe(0);
	});

	test("highly repetitive content is rejected by the DoS guard, not with a 500", {
		tag: "@contract",
	}, async ({ api }) => {
		// New. `_contains_suspicious_patterns` treats text whose unique-word
		// ratio is under 10% as a repetition attack (input_sanitizer.py:316-322)
		// and fails sanitization, which lands on the same early return as an
		// empty body.
		//
		// The point of the test is the *status*: a guard that raised instead of
		// returning an outcome would surface as a 500 through the endpoint's
		// blanket `except Exception` (auth_service.py:571-573), and a caller
		// batching articles would treat a poisoned input as an outage.
		const body = await extractTags(api, {
			title: "repeat",
			content: Array.from({ length: 300 }, () => "spam").join(" "),
		});

		expect(body.tags).toEqual([]);
		expect(body.confidence).toBe(0);
		expect(body.language).toBe("und");
	});

	test("Japanese text routes to the Japanese extractor", {
		tag: ["@contract", "@slow"],
	}, async ({ api }) => {
		// New, and the largest uncovered branch in the service: `_detect_language`
		// choosing "ja" is what selects fugashi/unidic-lite tokenisation and the
		// Japanese KeyBERT path (extract.py:390-397). The image bakes
		// `fugashi[unidic-lite]` in, so a broken dictionary install shows up
		// here — and only here — as an English-path answer or an empty result.
		const body = await extractTags(api, {
			title: "機械学習による自動タグ生成の仕組み",
			content:
				"この記事では、自然言語処理と機械学習を利用した自動タグ生成の実装について説明します。" +
				"日本語の形態素解析には fugashi と unidic を使用し、意味的な埋め込みには多言語の" +
				"文埋め込みモデルを利用しています。検索やレコメンドの精度向上に貢献します。",
		});

		expect(body.language).toBe("ja");
		expect(body.tags.length, "the Japanese pipeline produced no tags at all").toBeGreaterThan(0);
	});

	test("HTML markup never reaches the tag list", { tag: "@contract" }, async ({ api }) => {
		// New. Content arrives from RSS bodies, so markup is the normal case,
		// not the exotic one. Two independent defences run before extraction:
		// `_extract_readable_text` when the content looks like HTML
		// (input_sanitizer.py:232), and `DANGEROUS_ELEMENT_PATTERN` +
		// `nh3.clean`, which drop script/style/iframe/object/embed *with their
		// contents* (input_sanitizer.py:18-21, 288-300).
		//
		// A tag containing `<script>` would be stored, indexed and eventually
		// rendered somewhere — so this is the one assertion in the file that
		// is about a security property rather than about quality.
		const body = await extractTags(api, {
			title: "Semantic search with embeddings",
			content:
				"<div><p>Semantic search relies on vector embeddings computed by a " +
				"transformer model.</p><script>alert('xss')</script><p>Retrieval " +
				"quality depends on chunking strategy and on the embedding model " +
				"used for indexing documents.</p></div>",
		});

		for (const tag of body.tags) {
			expect(tag, `tag "${tag}" carries markup`).not.toMatch(/[<>]/);
			expect(tag.toLowerCase(), `tag "${tag}" came from a stripped element`).not.toContain(
				"script",
			);
			expect(tag.toLowerCase()).not.toContain("alert");
		}
	});

	test("unknown request fields are ignored rather than rejected", {
		tag: "@contract",
	}, async ({ api }) => {
		// New. `ExtractTagsRequest` declares no `model_config`, so Pydantic v2
		// applies `extra="ignore"` (auth_service.py:536-540). That is a
		// compatibility promise to recap-worker, the caller: it may add a field
		// to its request before tag-generator learns about it. If someone
		// tightens the model to `extra="forbid"`, every deploy ordering becomes
		// load-bearing, and this test is where that shows up.
		const body = await extractTags(api, {
			title: KEYWORD_TITLE,
			content: KEYWORD_CONTENT,
			article_id: "not-a-field-of-this-endpoint",
			tenant_id: "public",
		});
		expect(body.tags.length).toBeGreaterThan(0);
	});

	test("the same text extracts the same tags twice", {
		tag: ["@contract", "@slow"],
	}, async ({ api }) => {
		// New. `extract_tags_with_metrics` is a pure function of (title,
		// content): no cursor, no per-request seed, no state carried between
		// calls. That is what lets the pipeline be reprojected — pre-processor
		// re-tagging an article must not produce a different tag set from the
		// one alt-backend already stored.
		//
		// Two calls, one assertion on the *set*: order across a KeyBERT/MMR
		// merge is not part of the contract, membership is.
		const first = await extractTags(api, { title: KEYWORD_TITLE, content: KEYWORD_CONTENT });
		const second = await extractTags(api, { title: KEYWORD_TITLE, content: KEYWORD_CONTENT });

		expect([...second.tags].sort(), "extraction must be deterministic for identical input").toEqual(
			[...first.tags].sort(),
		);
		expect(second.confidence).toBe(first.confidence);
		expect(second.language).toBe(first.language);
	});
});

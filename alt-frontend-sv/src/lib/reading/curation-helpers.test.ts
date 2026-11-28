import { describe, expect, it } from "vitest";
import {
	AnnotationStance,
	type ArticleAnnotationItem,
	type ThemeCurationItem,
	ThemeCurationKind,
	type ThemeSentenceItem,
} from "#lib/connect/reading.js";
import {
	buildArticleAugurHref,
	buildThemeAugurHref,
	clampMicroText,
	deriveThemeCurationView,
	hashTargetSentence,
	MAX_CURATION_TEXT_LENGTH,
	selectRecallAnnotations,
	stanceLabel,
} from "./curation-helpers.js";

describe("curation-helpers", () => {
	describe("hashTargetSentence", () => {
		it("produces stable 16-hex-char SHA-256 prefix matching Go domain.HashTargetSentence", () => {
			// SHA-256("abc") = ba7816bf8f01cfea414140de5dae2223...
			expect(hashTargetSentence("abc")).toBe("ba7816bf8f01cfea");
			expect(hashTargetSentence("  abc  ")).toBe("ba7816bf8f01cfea");
			expect(hashTargetSentence("")).toBe("");
			expect(hashTargetSentence("   ")).toBe("");
			// UTF-8 Japanese text
			const jpHash = hashTargetSentence("知識昇華のテスト");
			expect(jpHash).toMatch(/^[0-9a-f]{16}$/);
			expect(jpHash).toBe(hashTargetSentence("  知識昇華のテスト\n"));
		});
	});

	describe("clampMicroText", () => {
		it("strips control characters, trims, and clamps by Unicode code points", () => {
			expect(clampMicroText("  hello\u0000world\nline2  ")).toBe(
				"helloworld\nline2",
			);
			const long = "あ".repeat(MAX_CURATION_TEXT_LENGTH + 25);
			const clamped = clampMicroText(long);
			expect([...clamped]).toHaveLength(MAX_CURATION_TEXT_LENGTH);
		});
	});

	describe("stanceLabel", () => {
		it("maps allow-listed stances to concise editorial labels", () => {
			expect(stanceLabel(AnnotationStance.SURPRISE)).toBe("Surprise");
			expect(stanceLabel(AnnotationStance.QUESTION)).toBe("Question");
			expect(stanceLabel(AnnotationStance.CONTRADICTION)).toBe("Counter");
			expect(stanceLabel(AnnotationStance.INSIGHT)).toBe("Insight");
			expect(stanceLabel(AnnotationStance.UNSPECIFIED)).toBe("Note");
		});
	});

	describe("deriveThemeCurationView", () => {
		const kp1: ThemeSentenceItem = {
			text: "Incremental linking cuts rebuild latency.",
			sourceArticleIds: ["s1"],
		};
		const kp2: ThemeSentenceItem = {
			text: "Remote cache invalidation causes tail spikes.",
			sourceArticleIds: ["s2"],
		};
		const kp3: ThemeSentenceItem = {
			text: "Noisy boilerplate line.",
			sourceArticleIds: ["s2"],
		};
		const oq1: ThemeSentenceItem = {
			text: "How do remote caches handle stale object files?",
			sourceArticleIds: ["s1"],
		};

		it("sorts pinned key points first, separates dismissed ones, and partitions questions/notes/insights", () => {
			const curations: ThemeCurationItem[] = [
				{
					curationId: "c-pin",
					themeId: "t-1",
					kind: ThemeCurationKind.PIN_KEY_POINT,
					targetHash: hashTargetSentence(kp2.text),
					targetText: kp2.text,
					userNote: "",
					augurConversationId: "",
					createdAt: new Date("2026-11-20T01:00:00Z"),
				},
				{
					curationId: "c-dismiss",
					themeId: "t-1",
					kind: ThemeCurationKind.DISMISS_KEY_POINT,
					targetHash: hashTargetSentence(kp3.text),
					targetText: kp3.text,
					userNote: "",
					augurConversationId: "",
					createdAt: new Date("2026-11-20T02:00:00Z"),
				},
				{
					curationId: "c-resolve",
					themeId: "t-1",
					kind: ThemeCurationKind.RESOLVE_QUESTION,
					targetHash: hashTargetSentence(oq1.text),
					targetText: oq1.text,
					userNote: "Content-addressed digests prevent staleness.",
					augurConversationId: "",
					createdAt: new Date("2026-11-20T03:00:00Z"),
				},
				{
					curationId: "c-add-q",
					themeId: "t-1",
					kind: ThemeCurationKind.ADD_QUESTION,
					targetHash: "",
					targetText: "Does this scale beyond 10M LOC?",
					userNote: "",
					augurConversationId: "",
					createdAt: new Date("2026-11-20T04:00:00Z"),
				},
				{
					curationId: "c-seed",
					themeId: "t-1",
					kind: ThemeCurationKind.ADD_SEED_NOTE,
					targetHash: "",
					targetText: "Compare mold vs lld on cold CI runners.",
					userNote: "",
					augurConversationId: "",
					createdAt: new Date("2026-11-20T05:00:00Z"),
				},
				{
					curationId: "c-augur",
					themeId: "t-1",
					kind: ThemeCurationKind.PROMOTE_AUGUR_INSIGHT,
					targetHash: "",
					targetText: "Why did cold link time drop?",
					userNote: "Parallel section merging removed the serial bottleneck.",
					augurConversationId: "conv-99",
					createdAt: new Date("2026-11-20T06:00:00Z"),
				},
			];

			const view = deriveThemeCurationView([kp1, kp2, kp3], [oq1], curations);

			expect(view.visibleKeyPoints.map((row) => row.item.text)).toEqual([
				kp2.text,
				kp1.text,
			]);
			expect(view.visibleKeyPoints[0]?.pinCuration?.curationId).toBe("c-pin");
			expect(view.visibleKeyPoints[1]?.pinCuration).toBeNull();
			expect(view.dismissedKeyPoints.map((row) => row.item.text)).toEqual([
				kp3.text,
			]);
			expect(view.dismissedKeyPoints[0]?.dismissCuration.curationId).toBe(
				"c-dismiss",
			);

			expect(view.openQuestionRows).toHaveLength(2);
			expect(view.openQuestionRows[0]?.resolvedCuration?.userNote).toBe(
				"Content-addressed digests prevent staleness.",
			);
			expect(view.openQuestionRows[1]?.isUserAdded).toBe(true);
			expect(view.openQuestionRows[1]?.userCurationId).toBe("c-add-q");

			expect(view.seedNotes).toHaveLength(1);
			expect(view.seedNotes[0]?.targetText).toBe(
				"Compare mold vs lld on cold CI runners.",
			);
			expect(view.promotedInsights).toHaveLength(1);
			expect(view.promotedInsights[0]?.augurConversationId).toBe("conv-99");
		});
	});

	describe("Augur href builders", () => {
		it("encodes theme and article deep links safely", () => {
			const themeHref = buildThemeAugurHref({
				question: "How does caching work?",
				themeId: "01938e82-7c00-7a7b-9b10-100000000001",
				themeTitle: "Compiler Toolchains",
			});
			const themeUrl = new URL(themeHref, "http://localhost");
			expect(themeUrl.pathname).toBe("/augur");
			expect(themeUrl.searchParams.get("q")).toBe("How does caching work?");
			expect(themeUrl.searchParams.get("context")).toBe("Compiler Toolchains");
			expect(themeUrl.searchParams.get("themeId")).toBe(
				"01938e82-7c00-7a7b-9b10-100000000001",
			);
			expect(themeUrl.searchParams.get("themeTitle")).toBe(
				"Compiler Toolchains",
			);

			const articleHref = buildArticleAugurHref({
				question: "Why is this faster?",
				articleTitle: "Fast Linker",
				articleId: "01938e82-7c00-7a7b-9b10-200000000002",
			});
			const articleUrl = new URL(articleHref, "http://localhost");
			expect(articleUrl.pathname).toBe("/augur");
			expect(articleUrl.searchParams.get("q")).toBe("Why is this faster?");
			expect(articleUrl.searchParams.get("context")).toBe("Fast Linker");
			expect(articleUrl.searchParams.get("articleId")).toBe(
				"01938e82-7c00-7a7b-9b10-200000000002",
			);
		});
	});

	describe("selectRecallAnnotations", () => {
		it("prioritizes question, contradiction, and insight takeaways up to limit", () => {
			const make = (
				id: string,
				stance: AnnotationStance,
				note: string,
			): ArticleAnnotationItem => ({
				annotationId: id,
				articleId: `art-${id}`,
				stance,
				excerpt: "",
				note,
				createdAt: new Date("2026-11-20T00:00:00Z"),
			});

			const selected = selectRecallAnnotations(
				[
					make("1", AnnotationStance.SURPRISE, "Surprising stat"),
					make("2", AnnotationStance.QUESTION, "Open question 1"),
					make("3", AnnotationStance.CONTRADICTION, "Counterpoint"),
					make("4", AnnotationStance.INSIGHT, "Core takeaway"),
					make("5", AnnotationStance.QUESTION, "Open question 2"),
				],
				3,
			);
			expect(selected.map((a) => a.annotationId)).toEqual(["2", "3", "4"]);
		});
	});
});

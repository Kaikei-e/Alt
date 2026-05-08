import { describe, expect, it } from "vitest";
import {
	buildAskTransitionMetadata,
	buildRecapTransitionMetadata,
	buildTransitionMetadata,
} from "./transition-metadata";
import type {
	ActTargetData,
	DecisionOptionData,
	KnowledgeLoopEntryData,
	WhyPayloadData,
} from "$lib/connect/knowledge_loop";

const NOW_ISO = "2026-05-08T12:00:00.000Z";

const baseWhy: WhyPayloadData = {
	kind: "source_why",
	text: "Test entry — semantic transition metadata.",
	confidence: 0.5,
	evidenceRefs: [],
};

function entry(
	overrides: Partial<KnowledgeLoopEntryData> = {},
): KnowledgeLoopEntryData {
	return {
		entryKey: "loop-entry-fixture",
		sourceItemKey: "article-fixture",
		proposedStage: "decide",
		surfaceBucket: "now",
		projectionRevision: 1,
		projectionSeqHiwater: 1,
		freshnessAt: NOW_ISO,
		whyPrimary: baseWhy,
		dismissState: "active",
		renderDepthHint: 2,
		loopPriority: "critical",
		decisionOptions: [],
		actTargets: [],
		surfacePlannerVersion: "v2",
		...overrides,
	} satisfies KnowledgeLoopEntryData;
}

const article: ActTargetData = {
	targetType: "article",
	targetRef: "article-fixture",
	route: "/articles/article-fixture",
	sourceUrl: "https://example.com/x",
};

const diffTarget: ActTargetData = {
	targetType: "diff",
	targetRef: "summary-version:v3",
};

const recap: ActTargetData = {
	targetType: "recap",
	targetRef: "recap-snapshot-1",
	route: "/recap/topic/recap-snapshot-1",
};

const opt = (
	intent: DecisionOptionData["intent"],
	actionId = `${intent}-1`,
	label?: string,
): DecisionOptionData => ({ actionId, intent, label: label ?? intent });

describe("buildTransitionMetadata", () => {
	it("returns presentedIntents from non-unspecified decision options", () => {
		const e = entry({
			decisionOptions: [opt("open"), opt("ask"), opt("save")],
			actTargets: [article],
		});
		const m = buildTransitionMetadata(e, opt("open"));
		expect(m.presentedIntents).toEqual(["open", "ask", "save"]);
	});

	it("attaches actedIntent + continueFlag=true for open with article target", () => {
		const e = entry({
			decisionOptions: [opt("open")],
			actTargets: [article],
		});
		const m = buildTransitionMetadata(e, opt("open", "open-1", "Open"));
		expect(m.actedIntent).toBe("open");
		expect(m.continueFlag).toBe(true);
		expect(m.targetType).toBe("article");
		expect(m.targetRef).toBe("article-fixture");
		expect(m.actionId).toBe("open-1");
	});

	it("save sets continueFlag=false and targets the article", () => {
		const e = entry({
			decisionOptions: [opt("save")],
			actTargets: [article],
		});
		const m = buildTransitionMetadata(e, opt("save"));
		expect(m.actedIntent).toBe("save");
		expect(m.continueFlag).toBe(false);
		expect(m.targetType).toBe("article");
	});

	it("compare picks the diff target when present", () => {
		const e = entry({
			decisionOptions: [opt("compare")],
			actTargets: [article, diffTarget],
		});
		const m = buildTransitionMetadata(e, opt("compare"));
		expect(m.actedIntent).toBe("compare");
		expect(m.targetType).toBe("diff");
		expect(m.targetRef).toBe("summary-version:v3");
		expect(m.continueFlag).toBe(false);
	});

	it("revisit targets the entry itself with continueFlag=true", () => {
		const e = entry({
			entryKey: "loop-entry-fixture",
			decisionOptions: [opt("revisit")],
			actTargets: [article],
		});
		const m = buildTransitionMetadata(e, opt("revisit"));
		expect(m.actedIntent).toBe("revisit");
		expect(m.targetType).toBe("entry");
		expect(m.targetRef).toBe("loop-entry-fixture");
		expect(m.continueFlag).toBe(true);
	});

	it("snooze targets the entry itself with continueFlag=false", () => {
		const e = entry({
			entryKey: "loop-entry-fixture",
			decisionOptions: [opt("snooze")],
		});
		const m = buildTransitionMetadata(e, opt("snooze"));
		expect(m.actedIntent).toBe("snooze");
		expect(m.targetType).toBe("entry");
		expect(m.targetRef).toBe("loop-entry-fixture");
		expect(m.continueFlag).toBe(false);
	});

	it("ask without conversation_id leaves target unset (caller upgrades after handshake)", () => {
		const e = entry({
			decisionOptions: [opt("ask")],
			actTargets: [article],
		});
		const m = buildTransitionMetadata(e, opt("ask"));
		expect(m.actedIntent).toBe("ask");
		expect(m.continueFlag).toBe(true);
		expect(m.targetType).toBeUndefined();
		expect(m.targetRef).toBeUndefined();
	});

	it("falls back to actionId='intent' when option.actionId is empty", () => {
		const e = entry({ decisionOptions: [opt("open", "")] });
		const m = buildTransitionMetadata(e, opt("open", ""));
		expect(m.actionId).toBe("open");
	});
});

describe("buildAskTransitionMetadata", () => {
	it("targets the new conversation with continueFlag=true", () => {
		const e = entry({ decisionOptions: [opt("ask")] });
		const m = buildAskTransitionMetadata(e, "conv-uuid-7-abc");
		expect(m.actedIntent).toBe("ask");
		expect(m.targetType).toBe("conversation");
		expect(m.targetRef).toBe("conv-uuid-7-abc");
		expect(m.continueFlag).toBe(true);
		expect(m.actionId).toBe("ask");
	});

	it("includes presentedIntents when the entry exposes options", () => {
		const e = entry({
			decisionOptions: [opt("open"), opt("ask"), opt("snooze")],
		});
		const m = buildAskTransitionMetadata(e, "conv");
		expect(m.presentedIntents).toEqual(["open", "ask", "snooze"]);
	});
});

describe("buildRecapTransitionMetadata", () => {
	it("uses the recap snapshot id as target_ref with continueFlag=true", () => {
		const e = entry({ actTargets: [recap] });
		const m = buildRecapTransitionMetadata(e, recap);
		expect(m.actedIntent).toBe("open");
		expect(m.targetType).toBe("recap");
		expect(m.targetRef).toBe("recap-snapshot-1");
		expect(m.continueFlag).toBe(true);
		expect(m.actionId).toBe("open-recap");
	});
});

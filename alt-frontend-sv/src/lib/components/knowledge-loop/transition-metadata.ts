import type {
	ActTargetData,
	DecisionIntentName,
	DecisionOptionData,
	KnowledgeLoopEntryData,
} from "$lib/connect/knowledge_loop";
import type { TransitionMetadata } from "$lib/hooks/useKnowledgeLoop.svelte";

export type SemanticTransitionMetadata = TransitionMetadata;

/**
 * Phase 2 (Knowledge Loop completion) — semantic Decide / Act feedback loop.
 *
 * `/loop` operations must return as `knowledge_loop.acted.v1` events whose
 * payload encodes *what intent the user selected* and *what target it acted
 * on*, not just a stage advance. Tile and workspace CTAs share this helper so
 * the BFF body shape is identical regardless of where the user clicks.
 *
 * Reproject-safe: this is a pure function of (entry, option). It does not
 * read any current projection row or external state. The projector consumes
 * the resulting payload to update `continue_context.recent_action_labels`,
 * `act_targets`, and Surface Planner v2's Continue signal.
 *
 * Reference: docs/plan/knowledge-loop-completion-02-semantic-decide-act.md
 *            docs/plan/knowledge-loop-canonical-contract.md §6.4.1
 */

const INTENTS_THAT_CONTINUE: ReadonlySet<DecisionIntentName> = new Set([
	"open",
	"ask",
	"revisit",
]);

function nonUnspecifiedIntents(
	options: ReadonlyArray<DecisionOptionData>,
): Exclude<DecisionIntentName, "unspecified">[] {
	const out: Exclude<DecisionIntentName, "unspecified">[] = [];
	for (const o of options) {
		if (o.intent !== "unspecified") {
			out.push(o.intent);
		}
	}
	return out;
}

/**
 * Build transition metadata for a presented Decide option (Open / Ask / Save
 * / Compare / Revisit / Snooze). Returns the metadata object the BFF will
 * forward to alt-backend, which writes it into the `knowledge_loop.acted.v1`
 * event payload.
 */
export function buildTransitionMetadata(
	entry: KnowledgeLoopEntryData,
	option: DecisionOptionData,
): TransitionMetadata {
	const presentedIntents = nonUnspecifiedIntents(entry.decisionOptions);
	const metadata: TransitionMetadata = {
		actionId: option.actionId || option.intent,
	};
	if (presentedIntents.length > 0) {
		metadata.presentedIntents = presentedIntents;
	}
	if (option.intent === "unspecified") {
		// We only attach acted_intent for known semantic verbs. Unknown intents
		// stay as a stage-only transition (the BFF rejects "unspecified" anyway).
		return metadata;
	}
	metadata.actedIntent = option.intent;
	metadata.continueFlag = INTENTS_THAT_CONTINUE.has(option.intent);

	const target = pickTargetForIntent(entry, option);
	if (target) {
		metadata.targetType = target.targetType;
		metadata.targetRef = target.targetRef;
	}
	return metadata;
}

/**
 * Build metadata for a successful Augur Ask handshake. The Loop transition
 * is fired only after the conversation is created so we have the
 * conversation_id to use as `target_ref`.
 */
export function buildAskTransitionMetadata(
	entry: KnowledgeLoopEntryData,
	conversationId: string,
): TransitionMetadata {
	const presentedIntents = nonUnspecifiedIntents(entry.decisionOptions);
	const metadata: TransitionMetadata = {
		actionId: "ask",
		actedIntent: "ask",
		targetType: "conversation",
		targetRef: conversationId,
		continueFlag: true,
	};
	if (presentedIntents.length > 0) {
		metadata.presentedIntents = presentedIntents;
	}
	return metadata;
}

/**
 * Build metadata for an Open Recap navigation. The recap target is seeded
 * by Surface Planner v2 from a `RecapTopicSnapshotted` event, so we use
 * the snapshot id as `target_ref`.
 */
export function buildRecapTransitionMetadata(
	entry: KnowledgeLoopEntryData,
	recapTarget: ActTargetData,
): TransitionMetadata {
	const presentedIntents = nonUnspecifiedIntents(entry.decisionOptions);
	const metadata: TransitionMetadata = {
		actionId: "open-recap",
		actedIntent: "open",
		targetType: "recap",
		targetRef: recapTarget.targetRef,
		continueFlag: true,
	};
	if (presentedIntents.length > 0) {
		metadata.presentedIntents = presentedIntents;
	}
	return metadata;
}

/**
 * Infer the semantic target from the entry's act_targets and the option's
 * intent. Phase 2 proto bump introduced `conversation` and `entry` enum
 * values so Snooze / Revisit have unambiguous target types instead of
 * falling back to article.
 */
function pickTargetForIntent(
	entry: KnowledgeLoopEntryData,
	option: DecisionOptionData,
): {
	targetType: Exclude<TransitionMetadata["targetType"], undefined>;
	targetRef: string;
} | null {
	switch (option.intent) {
		case "open":
		case "save": {
			const article = entry.actTargets.find((t) => t.targetType === "article");
			if (article) {
				return { targetType: "article", targetRef: article.targetRef };
			}
			return null;
		}
		case "compare": {
			const diff = entry.actTargets.find((t) => t.targetType === "diff");
			if (diff) {
				return { targetType: "diff", targetRef: diff.targetRef };
			}
			return null;
		}
		case "revisit":
		case "snooze":
			return { targetType: "entry", targetRef: entry.entryKey };
		case "ask":
			// Ask without a conversation_id (synchronous CTA) cannot meaningfully
			// target a conversation. Caller upgrades to buildAskTransitionMetadata
			// after the Augur handshake succeeds.
			return null;
		default:
			return null;
	}
}

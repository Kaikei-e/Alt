/**
 * Evening Pulse client for Connect-RPC
 *
 * Provides type-safe methods to call Evening Pulse endpoints.
 * Authentication is handled by the transport layer.
 */

import { createClient } from "@connectrpc/connect";
import type { Transport } from "@connectrpc/connect";
import {
	RecapService,
	type GetEveningPulseResponse,
	PulseStatus as ProtoPulseStatus,
	TopicRole as ProtoTopicRole,
	Confidence as ProtoConfidence,
} from "$lib/gen/alt/recap/v2/recap_pb";
import type {
	EveningPulse,
	PulseTopic,
	PulseStatus,
	TopicRole,
	Confidence,
	QuietDayInfo,
	WeeklyHighlight,
} from "$lib/schema/evening_pulse";

/**
 * Gets Evening Pulse data via Connect-RPC.
 *
 * @param transport - The Connect transport to use (must include auth)
 * @param date - Optional target date (YYYY-MM-DD format)
 * @returns Evening Pulse data
 */
export async function getEveningPulse(
	transport: Transport,
	date?: string,
): Promise<EveningPulse> {
	const client = createClient(RecapService, transport);
	const response = (await client.getEveningPulse({
		date,
	})) as GetEveningPulseResponse;

	return convertProtoToEveningPulse(response);
}

/**
 * Convert proto GetEveningPulseResponse to EveningPulse type.
 */
function convertProtoToEveningPulse(
	proto: GetEveningPulseResponse,
): EveningPulse {
	return {
		jobId: proto.jobId,
		date: proto.date,
		generatedAt: proto.generatedAt,
		status: convertProtoStatus(proto.status),
		topics: proto.topics.map(convertProtoTopic),
		quietDay: proto.quietDay ? convertProtoQuietDay(proto.quietDay) : undefined,
	};
}

/**
 * Convert proto PulseStatus to PulseStatus type.
 */
function convertProtoStatus(status: ProtoPulseStatus): PulseStatus {
	switch (status) {
		case ProtoPulseStatus.NORMAL:
			return "normal";
		case ProtoPulseStatus.PARTIAL:
			return "partial";
		case ProtoPulseStatus.QUIET_DAY:
			return "quiet_day";
		case ProtoPulseStatus.ERROR:
			return "error";
		default:
			return "error";
	}
}

/**
 * Convert proto TopicRole to TopicRole type.
 */
function convertProtoRole(role: ProtoTopicRole): TopicRole {
	switch (role) {
		case ProtoTopicRole.NEED_TO_KNOW:
			return "need_to_know";
		case ProtoTopicRole.TREND:
			return "trend";
		case ProtoTopicRole.SERENDIPITY:
			return "serendipity";
		default:
			return "need_to_know";
	}
}

/**
 * Convert proto Confidence to Confidence type.
 */
function convertProtoConfidence(confidence: ProtoConfidence): Confidence {
	switch (confidence) {
		case ProtoConfidence.HIGH:
			return "high";
		case ProtoConfidence.MEDIUM:
			return "medium";
		case ProtoConfidence.LOW:
			return "low";
		default:
			return "medium";
	}
}

/**
 * Convert proto PulseTopic to PulseTopic type.
 */
function convertProtoTopic(proto: {
	clusterId: bigint;
	role: ProtoTopicRole;
	title: string;
	rationale?: {
		text: string;
		confidence: ProtoConfidence;
	};
	articleCount: number;
	sourceCount: number;
	tier1Count?: number;
	timeAgo: string;
	trendMultiplier?: number;
	genre?: string;
	articleIds: string[];
}): PulseTopic {
	return {
		clusterId: Number(proto.clusterId),
		role: convertProtoRole(proto.role),
		title: proto.title,
		rationale: {
			text: proto.rationale?.text ?? "",
			confidence: convertProtoConfidence(
				proto.rationale?.confidence ?? ProtoConfidence.MEDIUM,
			),
		},
		articleCount: proto.articleCount,
		sourceCount: proto.sourceCount,
		tier1Count: proto.tier1Count,
		timeAgo: proto.timeAgo,
		trendMultiplier: proto.trendMultiplier,
		genre: proto.genre,
		articleIds: proto.articleIds,
	};
}

/**
 * Convert proto QuietDayInfo to QuietDayInfo type.
 */
function convertProtoQuietDay(proto: {
	message: string;
	weeklyHighlights: Array<{
		id: string;
		title: string;
		date: string;
		role: string;
	}>;
}): QuietDayInfo {
	return {
		message: proto.message,
		weeklyHighlights: proto.weeklyHighlights.map(
			(h): WeeklyHighlight => ({
				id: h.id,
				title: h.title,
				date: h.date,
				role: h.role,
			}),
		),
	};
}

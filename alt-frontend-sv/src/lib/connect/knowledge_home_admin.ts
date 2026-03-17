/**
 * KnowledgeHomeAdminService client for Connect-RPC
 *
 * Provides type-safe methods to call KnowledgeHomeAdminService endpoints.
 * Uses service-token authentication.
 */

import { createClient } from "@connectrpc/connect";
import type { Client, Transport } from "@connectrpc/connect";
import { KnowledgeHomeAdminService } from "$lib/gen/alt/knowledge_home/v1/knowledge_home_admin_pb";

type KnowledgeHomeAdminClient = Client<typeof KnowledgeHomeAdminService>;

/** Backfill job data */
export interface BackfillJobData {
	jobId: string;
	status: string;
	projectionVersion: number;
	totalEvents: number;
	processedEvents: number;
	errorMessage: string;
	createdAt: string;
	startedAt: string;
	completedAt: string;
}

/** Projection health data */
export interface ProjectionHealthData {
	activeVersion: number;
	checkpointSeq: number;
	lastUpdated: string;
	backfillJobs: BackfillJobData[];
}

/** Feature flags config data */
export interface FeatureFlagsConfigData {
	enableHomePage: boolean;
	enableTracking: boolean;
	enableProjectionV2: boolean;
	rolloutPercentage: number;
	enableRecallRail: boolean;
	enableLens: boolean;
	enableStreamUpdates: boolean;
	enableSupersedeUx: boolean;
}

/** Combined admin dashboard data */
export interface KnowledgeHomeAdminData {
	health: ProjectionHealthData | null;
	flags: FeatureFlagsConfigData | null;
}

function createAdminClient(transport: Transport): KnowledgeHomeAdminClient {
	return createClient(KnowledgeHomeAdminService, transport);
}

export async function getProjectionHealth(
	transport: Transport,
): Promise<ProjectionHealthData> {
	const client = createAdminClient(transport);
	const response = await client.getProjectionHealth({});
	return {
		activeVersion: response.activeVersion,
		checkpointSeq: Number(response.checkpointSeq),
		lastUpdated: response.lastUpdated,
		backfillJobs: response.backfillJobs.map((j) => ({
			jobId: j.jobId,
			status: j.status,
			projectionVersion: j.projectionVersion,
			totalEvents: j.totalEvents,
			processedEvents: j.processedEvents,
			errorMessage: j.errorMessage,
			createdAt: j.createdAt,
			startedAt: j.startedAt,
			completedAt: j.completedAt,
		})),
	};
}

export async function getFeatureFlags(
	transport: Transport,
): Promise<FeatureFlagsConfigData> {
	const client = createAdminClient(transport);
	const response = await client.getFeatureFlags({});
	return {
		enableHomePage: response.enableHomePage,
		enableTracking: response.enableTracking,
		enableProjectionV2: response.enableProjectionV2,
		rolloutPercentage: response.rolloutPercentage,
		enableRecallRail: response.enableRecallRail,
		enableLens: response.enableLens,
		enableStreamUpdates: response.enableStreamUpdates,
		enableSupersedeUx: response.enableSupersedeUx,
	};
}

export async function triggerBackfill(
	transport: Transport,
	projectionVersion: number,
): Promise<BackfillJobData | null> {
	const client = createAdminClient(transport);
	const response = await client.triggerBackfill({ projectionVersion });
	if (!response.job) return null;
	const j = response.job;
	return {
		jobId: j.jobId,
		status: j.status,
		projectionVersion: j.projectionVersion,
		totalEvents: j.totalEvents,
		processedEvents: j.processedEvents,
		errorMessage: j.errorMessage,
		createdAt: j.createdAt,
		startedAt: j.startedAt,
		completedAt: j.completedAt,
	};
}

export async function pauseBackfill(
	transport: Transport,
	jobId: string,
): Promise<void> {
	const client = createAdminClient(transport);
	await client.pauseBackfill({ jobId });
}

export async function resumeBackfill(
	transport: Transport,
	jobId: string,
): Promise<void> {
	const client = createAdminClient(transport);
	await client.resumeBackfill({ jobId });
}

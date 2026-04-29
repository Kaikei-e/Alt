import { createConnectTransport } from "@connectrpc/connect-web";
import { env } from "$env/dynamic/private";
import type {
	FeatureFlagsConfigData,
	ProjectionHealthData,
	SLOStatusData,
	ReprojectRunData,
	ReprojectDiffSummaryData,
	SystemMetricsData,
} from "$lib/connect/knowledge_home_admin";
import {
	getFeatureFlags,
	getProjectionHealth,
	pauseBackfill,
	resumeBackfill,
	triggerBackfill,
	emitArticleUrlBackfill,
	getSLOStatus,
	getSystemMetrics,
	listReprojectRuns,
	startReproject,
	compareReproject,
	swapReproject,
	rollbackReproject,
	runProjectionAudit,
	type ArticleUrlBackfillResultData,
	type ProjectionAuditData,
} from "$lib/connect/knowledge_home_admin";

const BFF_CONNECT_URL =
	env.BFF_CONNECT_URL || "http://alt-butterfly-facade:9250";

export interface KnowledgeHomeAdminSnapshot {
	health: ProjectionHealthData | null;
	flags: FeatureFlagsConfigData | null;
	sloStatus: SLOStatusData | null;
	reprojectRuns: ReprojectRunData[];
	systemMetrics: SystemMetricsData | null;
}

function createBffTransport(backendToken: string) {
	return createConnectTransport({
		baseUrl: BFF_CONNECT_URL,
		interceptors: [
			(next) => async (req) => {
				req.header.set("X-Alt-Backend-Token", backendToken);
				return next(req);
			},
		],
	});
}

export async function fetchKnowledgeHomeAdminSnapshot(
	backendToken: string,
): Promise<KnowledgeHomeAdminSnapshot> {
	const transport = createBffTransport(backendToken);
	const [health, flags, sloStatus, reprojectRuns, systemMetrics] =
		await Promise.all([
			getProjectionHealth(transport),
			getFeatureFlags(transport),
			getSLOStatus(transport).catch(() => null),
			listReprojectRuns(transport).catch(() => []),
			getSystemMetrics(transport).catch(() => null),
		]);

	return {
		health,
		flags,
		sloStatus,
		reprojectRuns,
		systemMetrics,
	};
}

export async function triggerKnowledgeHomeBackfill(
	backendToken: string,
	projectionVersion: number,
) {
	const transport = createBffTransport(backendToken);
	return triggerBackfill(transport, projectionVersion);
}

export async function emitKnowledgeHomeArticleUrlBackfill(
	backendToken: string,
	maxArticles: number,
	dryRun: boolean,
): Promise<ArticleUrlBackfillResultData> {
	const transport = createBffTransport(backendToken);
	return emitArticleUrlBackfill(transport, maxArticles, dryRun);
}

export async function pauseKnowledgeHomeBackfill(
	backendToken: string,
	jobId: string,
) {
	const transport = createBffTransport(backendToken);
	return pauseBackfill(transport, jobId);
}

export async function resumeKnowledgeHomeBackfill(
	backendToken: string,
	jobId: string,
) {
	const transport = createBffTransport(backendToken);
	return resumeBackfill(transport, jobId);
}

export async function startKnowledgeHomeReproject(
	backendToken: string,
	mode: string,
	fromVersion: string,
	toVersion: string,
	rangeStart?: string,
	rangeEnd?: string,
) {
	const transport = createBffTransport(backendToken);
	return startReproject(
		transport,
		mode,
		fromVersion,
		toVersion,
		rangeStart,
		rangeEnd,
	);
}

export async function compareKnowledgeHomeReproject(
	backendToken: string,
	reprojectRunId: string,
): Promise<ReprojectDiffSummaryData | null> {
	const transport = createBffTransport(backendToken);
	return compareReproject(transport, reprojectRunId);
}

export async function swapKnowledgeHomeReproject(
	backendToken: string,
	reprojectRunId: string,
) {
	const transport = createBffTransport(backendToken);
	return swapReproject(transport, reprojectRunId);
}

export async function rollbackKnowledgeHomeReproject(
	backendToken: string,
	reprojectRunId: string,
) {
	const transport = createBffTransport(backendToken);
	return rollbackReproject(transport, reprojectRunId);
}

export async function runKnowledgeHomeAudit(
	backendToken: string,
	projectionName: string,
	projectionVersion: string,
	sampleSize: number,
): Promise<ProjectionAuditData | null> {
	const transport = createBffTransport(backendToken);
	return runProjectionAudit(
		transport,
		projectionName,
		projectionVersion,
		sampleSize,
	);
}

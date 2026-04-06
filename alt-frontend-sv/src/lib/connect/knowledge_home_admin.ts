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

/** SLI status data */
export interface SLIStatusData {
	name: string;
	currentValue: number;
	targetValue: number;
	unit: string;
	status: string;
	errorBudgetConsumedPct: number;
}

/** Alert summary data */
export interface AlertSummaryData {
	alertName: string;
	severity: string;
	status: string;
	firedAt: string;
	description: string;
}

/** SLO status data */
export interface SLOStatusData {
	overallHealth: string;
	slis: SLIStatusData[];
	errorBudgetWindowDays: number;
	activeAlerts: AlertSummaryData[];
	computedAt: string;
}

/** Reproject run data */
export interface ReprojectRunData {
	reprojectRunId: string;
	projectionName: string;
	fromVersion: string;
	toVersion: string;
	initiatedBy: string;
	mode: string;
	status: string;
	rangeStart: string;
	rangeEnd: string;
	statsJson: string;
	diffSummaryJson: string;
	createdAt: string;
	startedAt: string;
	finishedAt: string;
}

/** Reproject diff summary data */
export interface ReprojectDiffSummaryData {
	fromItemCount: number;
	toItemCount: number;
	fromEmptyCount: number;
	toEmptyCount: number;
	fromAvgScore: number;
	toAvgScore: number;
	fromWhyDistribution: string;
	toWhyDistribution: string;
}

/** System metrics data */
export interface SystemMetricsData {
	projector: ProjectorMetricsData;
	handler: HandlerMetricsData;
	tracking: TrackingMetricsData;
	stream: StreamMetricsData;
	correctness: CorrectnessMetricsData;
	sovereign: SovereignMetricsData;
	recall: RecallMetricsData;
	serviceHealth: ServiceHealthStatusData[];
}

export interface ProjectorMetricsData {
	eventsProcessed: number;
	lagSeconds: number;
	batchDurationMsP50: number;
	batchDurationMsP95: number;
	batchDurationMsP99: number;
	errors: number;
}

export interface HandlerMetricsData {
	pagesServed: number;
	pagesDegraded: number;
	degradedRatePct: number;
}

export interface TrackingMetricsData {
	itemsExposed: number;
	itemsOpened: number;
	itemsDismissed: number;
	openRatePct: number;
	dismissRatePct: number;
}

export interface StreamMetricsData {
	connectionsTotal: number;
	disconnectsTotal: number;
	reconnectsTotal: number;
	deliveriesTotal: number;
	disconnectRatePct: number;
}

export interface CorrectnessMetricsData {
	emptyResponses: number;
	malformedWhy: number;
	orphanItems: number;
	supersedeMismatch: number;
	requestsTotal: number;
	correctnessScorePct: number;
}

export interface SovereignMetricsData {
	mutationsApplied: number;
	mutationsErrors: number;
	mutationDurationMsP50: number;
	mutationDurationMsP95: number;
	errorRatePct: number;
}

export interface RecallMetricsData {
	signalsAppended: number;
	signalErrors: number;
	candidatesGenerated: number;
	candidatesEmpty: number;
	usersProcessed: number;
	projectorDurationMsP50: number;
	projectorDurationMsP95: number;
}

export interface ServiceHealthStatusData {
	serviceName: string;
	endpoint: string;
	status: string;
	latencyMs: number;
	checkedAt: string;
	errorMessage: string;
}

/** Combined admin dashboard data */
export interface KnowledgeHomeAdminData {
	health: ProjectionHealthData | null;
	flags: FeatureFlagsConfigData | null;
	sloStatus: SLOStatusData | null;
	reprojectRuns: ReprojectRunData[];
	systemMetrics: SystemMetricsData | null;
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

export async function getSLOStatus(
	transport: Transport,
): Promise<SLOStatusData> {
	const client = createAdminClient(transport);
	const response = await client.getSLOStatus({});
	return {
		overallHealth: response.overallHealth,
		slis: response.slis.map((s) => ({
			name: s.name,
			currentValue: s.currentValue,
			targetValue: s.targetValue,
			unit: s.unit,
			status: s.status,
			errorBudgetConsumedPct: s.errorBudgetConsumedPct,
		})),
		errorBudgetWindowDays: response.errorBudgetWindowDays,
		activeAlerts: response.activeAlerts.map((a) => ({
			alertName: a.alertName,
			severity: a.severity,
			status: a.status,
			firedAt: a.firedAt,
			description: a.description,
		})),
		computedAt: response.computedAt,
	};
}

export async function getSystemMetrics(
	transport: Transport,
): Promise<SystemMetricsData> {
	const client = createAdminClient(transport);
	const response = await client.getSystemMetrics({});
	const p = response.projector;
	const h = response.handler;
	const t = response.tracking;
	const s = response.stream;
	const c = response.correctness;
	const sv = response.sovereign;
	const r = response.recall;
	return {
		projector: {
			eventsProcessed: Number(p?.eventsProcessed ?? 0n),
			lagSeconds: p?.lagSeconds ?? 0,
			batchDurationMsP50: p?.batchDurationMsP50 ?? 0,
			batchDurationMsP95: p?.batchDurationMsP95 ?? 0,
			batchDurationMsP99: p?.batchDurationMsP99 ?? 0,
			errors: Number(p?.errors ?? 0n),
		},
		handler: {
			pagesServed: Number(h?.pagesServed ?? 0n),
			pagesDegraded: Number(h?.pagesDegraded ?? 0n),
			degradedRatePct: h?.degradedRatePct ?? 0,
		},
		tracking: {
			itemsExposed: Number(t?.itemsExposed ?? 0n),
			itemsOpened: Number(t?.itemsOpened ?? 0n),
			itemsDismissed: Number(t?.itemsDismissed ?? 0n),
			openRatePct: t?.openRatePct ?? 0,
			dismissRatePct: t?.dismissRatePct ?? 0,
		},
		stream: {
			connectionsTotal: Number(s?.connectionsTotal ?? 0n),
			disconnectsTotal: Number(s?.disconnectsTotal ?? 0n),
			reconnectsTotal: Number(s?.reconnectsTotal ?? 0n),
			deliveriesTotal: Number(s?.deliveriesTotal ?? 0n),
			disconnectRatePct: s?.disconnectRatePct ?? 0,
		},
		correctness: {
			emptyResponses: Number(c?.emptyResponses ?? 0n),
			malformedWhy: Number(c?.malformedWhy ?? 0n),
			orphanItems: Number(c?.orphanItems ?? 0n),
			supersedeMismatch: Number(c?.supersedeMismatch ?? 0n),
			requestsTotal: Number(c?.requestsTotal ?? 0n),
			correctnessScorePct: c?.correctnessScorePct ?? 0,
		},
		sovereign: {
			mutationsApplied: Number(sv?.mutationsApplied ?? 0n),
			mutationsErrors: Number(sv?.mutationsErrors ?? 0n),
			mutationDurationMsP50: sv?.mutationDurationMsP50 ?? 0,
			mutationDurationMsP95: sv?.mutationDurationMsP95 ?? 0,
			errorRatePct: sv?.errorRatePct ?? 0,
		},
		recall: {
			signalsAppended: Number(r?.signalsAppended ?? 0n),
			signalErrors: Number(r?.signalErrors ?? 0n),
			candidatesGenerated: Number(r?.candidatesGenerated ?? 0n),
			candidatesEmpty: Number(r?.candidatesEmpty ?? 0n),
			usersProcessed: Number(r?.usersProcessed ?? 0n),
			projectorDurationMsP50: r?.projectorDurationMsP50 ?? 0,
			projectorDurationMsP95: r?.projectorDurationMsP95 ?? 0,
		},
		serviceHealth: response.serviceHealth.map((sh) => ({
			serviceName: sh.serviceName,
			endpoint: sh.endpoint,
			status: sh.status,
			latencyMs: Number(sh.latencyMs),
			checkedAt: sh.checkedAt,
			errorMessage: sh.errorMessage,
		})),
	};
}

export async function listReprojectRuns(
	transport: Transport,
	limit = 20,
): Promise<ReprojectRunData[]> {
	const client = createAdminClient(transport);
	const response = await client.listReprojectRuns({ limit });
	return response.runs.map(convertReprojectRun);
}

export async function startReproject(
	transport: Transport,
	mode: string,
	fromVersion: string,
	toVersion: string,
	rangeStart?: string,
	rangeEnd?: string,
): Promise<ReprojectRunData | null> {
	const client = createAdminClient(transport);
	const response = await client.startReproject({
		mode,
		fromVersion,
		toVersion,
		rangeStart,
		rangeEnd,
	});
	if (!response.run) return null;
	return convertReprojectRun(response.run);
}

export async function compareReproject(
	transport: Transport,
	reprojectRunId: string,
): Promise<ReprojectDiffSummaryData | null> {
	const client = createAdminClient(transport);
	const response = await client.compareReproject({ reprojectRunId });
	if (!response.diff) return null;
	return {
		fromItemCount: Number(response.diff.fromItemCount),
		toItemCount: Number(response.diff.toItemCount),
		fromEmptyCount: Number(response.diff.fromEmptyCount),
		toEmptyCount: Number(response.diff.toEmptyCount),
		fromAvgScore: response.diff.fromAvgScore,
		toAvgScore: response.diff.toAvgScore,
		fromWhyDistribution: response.diff.fromWhyDistribution,
		toWhyDistribution: response.diff.toWhyDistribution,
	};
}

export async function swapReproject(
	transport: Transport,
	reprojectRunId: string,
): Promise<void> {
	const client = createAdminClient(transport);
	await client.swapReproject({ reprojectRunId });
}

export async function rollbackReproject(
	transport: Transport,
	reprojectRunId: string,
): Promise<void> {
	const client = createAdminClient(transport);
	await client.rollbackReproject({ reprojectRunId });
}

/** Projection audit result data */
export interface ProjectionAuditData {
	auditId: string;
	projectionName: string;
	projectionVersion: string;
	checkedAt: string;
	sampleSize: number;
	mismatchCount: number;
	detailsJson: string;
}

export async function runProjectionAudit(
	transport: Transport,
	projectionName: string,
	projectionVersion: string,
	sampleSize: number,
): Promise<ProjectionAuditData | null> {
	const client = createAdminClient(transport);
	const response = await client.runProjectionAudit({
		projectionName,
		projectionVersion,
		sampleSize,
	});
	if (!response.audit) return null;
	return {
		auditId: response.audit.auditId,
		projectionName: response.audit.projectionName,
		projectionVersion: response.audit.projectionVersion,
		checkedAt: response.audit.checkedAt,
		sampleSize: response.audit.sampleSize,
		mismatchCount: response.audit.mismatchCount,
		detailsJson: response.audit.detailsJson,
	};
}

function convertReprojectRun(r: {
	reprojectRunId: string;
	projectionName: string;
	fromVersion: string;
	toVersion: string;
	initiatedBy: string;
	mode: string;
	status: string;
	rangeStart: string;
	rangeEnd: string;
	statsJson: string;
	diffSummaryJson: string;
	createdAt: string;
	startedAt: string;
	finishedAt: string;
}): ReprojectRunData {
	return {
		reprojectRunId: r.reprojectRunId,
		projectionName: r.projectionName,
		fromVersion: r.fromVersion,
		toVersion: r.toVersion,
		initiatedBy: r.initiatedBy,
		mode: r.mode,
		status: r.status,
		rangeStart: r.rangeStart,
		rangeEnd: r.rangeEnd,
		statsJson: r.statsJson,
		diffSummaryJson: r.diffSummaryJson,
		createdAt: r.createdAt,
		startedAt: r.startedAt,
		finishedAt: r.finishedAt,
	};
}

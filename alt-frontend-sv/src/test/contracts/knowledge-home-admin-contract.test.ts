/**
 * Knowledge Home Admin API Contract Tests
 *
 * Validates Phase 5 admin proto schema conformance for
 * Reproject, SLO, and Audit operations.
 */
import { describe, it, expect } from "vitest";
import { create, toBinary, fromBinary } from "@bufbuild/protobuf";
import {
	StartReprojectRequestSchema,
	StartReprojectResponseSchema,
	GetSLOStatusResponseSchema,
	SLIStatusSchema,
	AlertSummarySchema,
	ReprojectRunSchema,
	ReprojectDiffSummarySchema,
	ListReprojectRunsResponseSchema,
	RunProjectionAuditResponseSchema,
	ProjectionAuditSchema,
} from "$lib/gen/alt/knowledge_home/v1/knowledge_home_admin_pb";

describe("Knowledge Home Admin API Contract - Phase 5", () => {
	describe("Reproject", () => {
		it("StartReprojectRequest validates mode values", () => {
			const validModes = ["dry_run", "user_subset", "time_range", "full"];
			for (const mode of validModes) {
				const req = create(StartReprojectRequestSchema, {
					mode,
					fromVersion: "1",
					toVersion: "2",
				});
				expect(req.mode).toBe(mode);
			}
		});

		it("StartReprojectRequest supports time_range with range fields", () => {
			const req = create(StartReprojectRequestSchema, {
				mode: "time_range",
				fromVersion: "1",
				toVersion: "2",
				rangeStart: "2026-03-01T00:00:00Z",
				rangeEnd: "2026-03-18T00:00:00Z",
			});
			expect(req.rangeStart).toBe("2026-03-01T00:00:00Z");
			expect(req.rangeEnd).toBe("2026-03-18T00:00:00Z");
		});

		it("ReprojectRun validates status values", () => {
			const validStatuses = [
				"pending",
				"running",
				"validating",
				"swappable",
				"swapped",
				"failed",
				"cancelled",
			];
			for (const status of validStatuses) {
				const run = create(ReprojectRunSchema, {
					reprojectRunId: "550e8400-e29b-41d4-a716-446655440000",
					projectionName: "knowledge-home",
					fromVersion: "1",
					toVersion: "2",
					mode: "full",
					status,
					createdAt: "2026-03-18T12:00:00Z",
				});
				expect(run.status).toBe(status);
			}
		});

		it("ReprojectDiffSummary round-trips correctly", () => {
			const original = create(ReprojectDiffSummarySchema, {
				fromItemCount: BigInt(1000),
				toItemCount: BigInt(1050),
				fromEmptyCount: BigInt(5),
				toEmptyCount: BigInt(3),
				fromAvgScore: 0.72,
				toAvgScore: 0.75,
				fromWhyDistribution: '{"new_unread":500,"tag_hotspot":300}',
				toWhyDistribution: '{"new_unread":520,"tag_hotspot":330}',
			});

			const binary = toBinary(ReprojectDiffSummarySchema, original);
			const deserialized = fromBinary(ReprojectDiffSummarySchema, binary);

			expect(deserialized.fromItemCount).toBe(BigInt(1000));
			expect(deserialized.toItemCount).toBe(BigInt(1050));
			expect(deserialized.toAvgScore).toBeCloseTo(0.75);
		});

		it("ListReprojectRunsResponse contains runs array", () => {
			const response = create(ListReprojectRunsResponseSchema, {
				runs: [
					{
						reprojectRunId: "run-1",
						projectionName: "knowledge-home",
						fromVersion: "1",
						toVersion: "2",
						mode: "full",
						status: "swappable",
						createdAt: "2026-03-18T12:00:00Z",
					},
					{
						reprojectRunId: "run-2",
						projectionName: "knowledge-home",
						fromVersion: "1",
						toVersion: "2",
						mode: "dry_run",
						status: "swapped",
						createdAt: "2026-03-17T12:00:00Z",
					},
				],
			});

			expect(response.runs).toHaveLength(2);
			expect(response.runs[0].status).toBe("swappable");
			expect(response.runs[1].mode).toBe("dry_run");
		});
	});

	describe("SLO Status", () => {
		it("GetSLOStatusResponse conforms to schema", () => {
			const response = create(GetSLOStatusResponseSchema, {
				overallHealth: "healthy",
				slis: [
					{
						name: "availability",
						currentValue: 99.7,
						targetValue: 99.5,
						unit: "%",
						status: "meeting",
						errorBudgetConsumedPct: 40.0,
					},
					{
						name: "freshness",
						currentValue: 120,
						targetValue: 300,
						unit: "seconds",
						status: "meeting",
						errorBudgetConsumedPct: 15.0,
					},
					{
						name: "action_durability",
						currentValue: 99.9,
						targetValue: 99.0,
						unit: "%",
						status: "meeting",
						errorBudgetConsumedPct: 5.0,
					},
					{
						name: "stream_continuity",
						currentValue: 98.5,
						targetValue: 95.0,
						unit: "%",
						status: "meeting",
						errorBudgetConsumedPct: 10.0,
					},
					{
						name: "correctness_proxy",
						currentValue: 99.99,
						targetValue: 99.9,
						unit: "%",
						status: "meeting",
						errorBudgetConsumedPct: 1.0,
					},
				],
				errorBudgetWindowDays: 30,
				activeAlerts: [],
				computedAt: "2026-03-18T12:00:00Z",
			});

			expect(response.slis).toHaveLength(5);
			expect(response.overallHealth).toBe("healthy");
			expect(response.errorBudgetWindowDays).toBe(30);
		});

		it("validates SLI status values", () => {
			const validStatuses = ["meeting", "burning", "breached"];
			for (const status of validStatuses) {
				const sli = create(SLIStatusSchema, {
					name: "availability",
					currentValue: 99.0,
					targetValue: 99.5,
					unit: "%",
					status,
					errorBudgetConsumedPct: 50.0,
				});
				expect(sli.status).toBe(status);
			}
		});

		it("validates overall health values", () => {
			const healthValues = ["healthy", "at_risk", "breaching"];
			for (const health of healthValues) {
				const response = create(GetSLOStatusResponseSchema, {
					overallHealth: health,
					slis: [],
					errorBudgetWindowDays: 30,
					computedAt: "2026-03-18T12:00:00Z",
				});
				expect(response.overallHealth).toBe(health);
			}
		});

		it("AlertSummary includes required fields", () => {
			const alert = create(AlertSummarySchema, {
				alertName: "KnowledgeHomeAvailabilityBurnRateHigh",
				severity: "page",
				status: "firing",
				firedAt: "2026-03-18T11:00:00Z",
				description: "Error rate exceeds 14.4x burn rate",
			});

			expect(alert.alertName).toBeTruthy();
			expect(alert.severity).toBe("page");
			expect(alert.firedAt).toBeTruthy();
		});
	});

	describe("Projection Audit", () => {
		it("RunProjectionAuditResponse conforms to schema", () => {
			const response = create(RunProjectionAuditResponseSchema, {
				audit: {
					auditId: "550e8400-e29b-41d4-a716-446655440000",
					projectionName: "knowledge-home",
					projectionVersion: "2",
					checkedAt: "2026-03-18T12:00:00Z",
					sampleSize: 100,
					mismatchCount: 2,
					detailsJson: '{"mismatched_items":["article:1","article:2"]}',
				},
			});

			expect(response.audit).toBeDefined();
			expect(response.audit?.sampleSize).toBe(100);
			expect(response.audit?.mismatchCount).toBe(2);
		});

		it("ProjectionAudit round-trips correctly", () => {
			const original = create(ProjectionAuditSchema, {
				auditId: "test-audit-id",
				projectionName: "knowledge-home",
				projectionVersion: "1",
				checkedAt: "2026-03-18T12:00:00Z",
				sampleSize: 50,
				mismatchCount: 0,
				detailsJson: "{}",
			});

			const binary = toBinary(ProjectionAuditSchema, original);
			const deserialized = fromBinary(ProjectionAuditSchema, binary);

			expect(deserialized.sampleSize).toBe(50);
			expect(deserialized.mismatchCount).toBe(0);
		});
	});
});

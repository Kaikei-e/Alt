/**
 * Factory for Knowledge Home Admin mock data (sovereign + audit).
 */

export const SOVEREIGN_ADMIN_SNAPSHOT = {
	storageStats: [
		{
			table_name: "knowledge_events",
			row_count: 786652,
			total_size: "760 MB",
			total_bytes: 796917760,
			is_partitioned: true,
		},
		{
			table_name: "knowledge_home_items",
			row_count: 15000,
			total_size: "12 MB",
			total_bytes: 12582912,
			is_partitioned: false,
		},
		{
			table_name: "today_digest_view",
			row_count: 30,
			total_size: "128 kB",
			total_bytes: 131072,
			is_partitioned: false,
		},
		{
			table_name: "recall_candidate_view",
			row_count: 500,
			total_size: "2 MB",
			total_bytes: 2097152,
			is_partitioned: false,
		},
	],
	snapshots: [
		{
			snapshotId: "550e8400-e29b-41d4-a716-446655440000",
			snapshotType: "full",
			projectionVersion: 1,
			projectorBuildRef: "abc123",
			schemaVersion: "00009",
			snapshotAt: "2026-03-25T12:00:00Z",
			eventSeqBoundary: 786652,
			snapshotDataPath: "/tmp/snapshots/snapshot_20260325_120000",
			itemsRowCount: 15000,
			itemsChecksum: "sha256:abcdef1234567890",
			digestRowCount: 30,
			digestChecksum: "sha256:digest1234567890",
			recallRowCount: 500,
			recallChecksum: "sha256:recall1234567890",
			createdAt: "2026-03-25T12:00:01Z",
			status: "valid",
		},
	],
	latestSnapshot: {
		snapshotId: "550e8400-e29b-41d4-a716-446655440000",
		snapshotType: "full",
		projectionVersion: 1,
		projectorBuildRef: "abc123",
		schemaVersion: "00009",
		snapshotAt: "2026-03-25T12:00:00Z",
		eventSeqBoundary: 786652,
		snapshotDataPath: "/tmp/snapshots/snapshot_20260325_120000",
		itemsRowCount: 15000,
		itemsChecksum: "sha256:abcdef1234567890",
		digestRowCount: 30,
		digestChecksum: "sha256:digest1234567890",
		recallRowCount: 500,
		recallChecksum: "sha256:recall1234567890",
		createdAt: "2026-03-25T12:00:01Z",
		status: "valid",
	},
	retentionLogs: [
		{
			logId: "660e8400-e29b-41d4-a716-446655440001",
			runAt: "2026-03-25T10:00:00Z",
			action: "export",
			targetTable: "knowledge_events",
			targetPartition: "knowledge_events_y2025m11",
			rowsAffected: 42000,
			archivePath: "/tmp/archives/knowledge_events_y2025m11_20260325.jsonl.gz",
			checksum: "sha256:archivechecksum",
			dryRun: false,
			status: "exported",
			errorMessage: "",
		},
	],
	eligiblePartitions: [
		{
			table: "knowledge_events",
			eligible: [
				{
					name: "knowledge_events_y2025m11",
					rangeStart: "2025-11-01T00:00:00Z",
					rangeEnd: "2025-12-01T00:00:00Z",
					rowCount: 42000,
					sizeBytes: 52428800,
				},
			],
		},
	],
};

export const KNOWLEDGE_HOME_ADMIN_SNAPSHOT = {
	health: {
		activeVersion: 1,
		checkpointSeq: 786652,
		lastUpdated: "2026-03-25T12:00:00Z",
		backfillJobs: [],
	},
	flags: {
		enableHomePage: true,
		enableTracking: true,
		enableProjectionV2: false,
		rolloutPercentage: 100,
		enableRecallRail: true,
		enableLens: true,
		enableStreamUpdates: false,
		enableSupersedeUx: false,
	},
	sloStatus: null,
	reprojectRuns: [],
};

export const AUDIT_RESULT = {
	ok: true,
	audit: {
		auditId: "770e8400-e29b-41d4-a716-446655440002",
		projectionName: "knowledge_home",
		projectionVersion: "2",
		checkedAt: "2026-03-25T13:00:00Z",
		sampleSize: 100,
		mismatchCount: 2,
		detailsJson: '{"mismatched_items":["article:1","article:2"]}',
	},
};

export const RETENTION_RUN_RESULT = {
	ok: true,
	result: {
		dry_run: true,
		actions: [
			{
				action: "export",
				table: "knowledge_events",
				partition: "knowledge_events_y2025m11",
				rows: 0,
				status: "dry_run",
			},
		],
	},
};

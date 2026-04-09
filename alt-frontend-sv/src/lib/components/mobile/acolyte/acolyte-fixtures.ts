import type {
	AcolyteReportSummary,
	AcolyteReport,
	AcolyteSection,
	AcolyteVersionSummary,
} from "$lib/connect/acolyte";

export const MOCK_REPORT_SUMMARIES: AcolyteReportSummary[] = [
	{
		reportId: "rpt-001",
		title: "AI Semiconductor Supply Chain Analysis",
		reportType: "weekly_briefing",
		currentVersion: 2,
		latestRunStatus: "succeeded",
		createdAt: "2026-04-07T09:00:00Z",
	},
	{
		reportId: "rpt-002",
		title: "Q2 Market Outlook",
		reportType: "market_analysis",
		currentVersion: 1,
		latestRunStatus: "running",
		createdAt: "2026-04-05T14:30:00Z",
	},
	{
		reportId: "rpt-003",
		title: "Emerging LLM Architectures",
		reportType: "tech_review",
		currentVersion: 1,
		latestRunStatus: "failed",
		createdAt: "2026-04-03T11:00:00Z",
	},
	{
		reportId: "rpt-004",
		title: "Custom Industry Report",
		reportType: "custom",
		currentVersion: 0,
		latestRunStatus: "draft",
		createdAt: "2026-04-01T08:00:00Z",
	},
];

export const MOCK_REPORT: AcolyteReport = {
	reportId: "rpt-001",
	title: "AI Semiconductor Supply Chain Analysis",
	reportType: "weekly_briefing",
	currentVersion: 2,
	createdAt: "2026-04-07T09:00:00Z",
};

export const MOCK_SECTIONS: AcolyteSection[] = [
	{
		sectionKey: "overview",
		currentVersion: 2,
		displayOrder: 1,
		body: "## Overview\n\nThe AI semiconductor supply chain has shown significant shifts.",
		citationsJson: "[]",
	},
	{
		sectionKey: "market_trends",
		currentVersion: 1,
		displayOrder: 2,
		body: "## Market Trends\n\nTSMC expanded 3nm capacity by 40%.",
		citationsJson: "[]",
	},
	{
		sectionKey: "technology_landscape",
		currentVersion: 1,
		displayOrder: 3,
		body: "## Technology Landscape\n\nNew architectures are emerging.",
		citationsJson: "[]",
	},
];

export const MOCK_VERSIONS: AcolyteVersionSummary[] = [
	{
		versionNo: 2,
		changeReason: "Full pipeline run",
		createdAt: "2026-04-07T09:30:00Z",
		changeItems: [
			{
				fieldName: "overview",
				changeKind: "updated",
				oldFingerprint: "abc",
				newFingerprint: "def",
			},
			{
				fieldName: "market_trends",
				changeKind: "regenerated",
				oldFingerprint: "ghi",
				newFingerprint: "jkl",
			},
		],
	},
	{
		versionNo: 1,
		changeReason: "Initial generation",
		createdAt: "2026-04-05T14:30:00Z",
		changeItems: [
			{
				fieldName: "overview",
				changeKind: "added",
				oldFingerprint: "",
				newFingerprint: "abc",
			},
			{
				fieldName: "market_trends",
				changeKind: "added",
				oldFingerprint: "",
				newFingerprint: "ghi",
			},
		],
	},
];

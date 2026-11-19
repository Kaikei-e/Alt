export interface MockThemeSummary {
	themeId: string;
	title: string;
	latestVersion: number;
	articleCount: number;
	updatedAt: string;
	lastOpenedAt?: string;
	lastOpenedVersion: number;
}

export interface MockThemePage {
	themeId: string;
	title: string;
	version: number;
	latestVersion: number;
	createdAt: string;
	keyPoints: Array<{ text: string; sourceArticleIds: string[] }>;
	timeline: Array<{
		date: string;
		text: string;
		sourceArticleIds: string[];
	}>;
	whatChanged: Array<{ text: string; sourceArticleIds: string[] }>;
	openQuestions: Array<{ text: string; sourceArticleIds: string[] }>;
	sources: Array<{ articleId: string; title: string; url: string }>;
	versions: Array<{
		version: number;
		createdAt: string;
		articleCount: number;
	}>;
}

export interface MockThemeRecord {
	summary: MockThemeSummary;
	versions: Map<number, MockThemePage>;
}

export const THEME_UNOPENED_ID = "01938e82-7c00-7a7b-9b10-100000000001";
export const THEME_OPENED_ID = "01938e82-7c00-7a7b-9b10-100000000002";

const SRC_COMPILER_1 = "01938e82-7c00-7a7b-9b10-200000000001";
const SRC_COMPILER_2 = "01938e82-7c00-7a7b-9b10-200000000002";
const SRC_COMPILER_3 = "01938e82-7c00-7a7b-9b10-200000000003";
const SRC_STORAGE_1 = "01938e82-7c00-7a7b-9b10-200000000004";
const SRC_STORAGE_2 = "01938e82-7c00-7a7b-9b10-200000000005";

export function createInitialThemesStore(): Map<string, MockThemeRecord> {
	const compilerVersionsMeta = [
		{
			version: 2,
			createdAt: "2026-11-18T10:00:00Z",
			articleCount: 3,
		},
		{
			version: 1,
			createdAt: "2026-11-16T09:00:00Z",
			articleCount: 2,
		},
	];

	const compilerV2: MockThemePage = {
		themeId: THEME_UNOPENED_ID,
		title: "Compiler Toolchains",
		version: 2,
		latestVersion: 2,
		createdAt: "2026-11-18T10:00:00Z",
		keyPoints: [
			{
				text: "Incremental linking cuts full rebuild latency across workspace crates.",
				sourceArticleIds: [SRC_COMPILER_1, SRC_COMPILER_2],
			},
			{
				text: "Content-addressed artifact caches prevent duplicate codegen runs.",
				sourceArticleIds: [SRC_COMPILER_3],
			},
		],
		timeline: [
			{
				date: "2026-11-15",
				text: "Baseline single-threaded linker benchmarks published.",
				sourceArticleIds: [SRC_COMPILER_1],
			},
			{
				date: "2026-11-18",
				text: "Parallel symbol resolution enabled by default in nightly builds.",
				sourceArticleIds: [SRC_COMPILER_2],
			},
		],
		whatChanged: [
			{
				text: "Parallel symbol resolution reduced incremental link times by half.",
				sourceArticleIds: [SRC_COMPILER_2],
			},
		],
		openQuestions: [
			{
				text: "How should distributed build caches expire stale debug symbol archives?",
				sourceArticleIds: [SRC_COMPILER_3],
			},
		],
		sources: [
			{
				articleId: SRC_COMPILER_1,
				title: "Incremental Linker Benchmarks",
				url: "https://toolchain.example/incremental-linker",
			},
			{
				articleId: SRC_COMPILER_2,
				title: "Parallel Symbol Resolution Notes",
				url: "https://runtime.example/parallel-symbols",
			},
			{
				articleId: SRC_COMPILER_3,
				title: "",
				url: "https://cache.example/artifact-store",
			},
		],
		versions: compilerVersionsMeta,
	};

	const compilerV1: MockThemePage = {
		themeId: THEME_UNOPENED_ID,
		title: "Compiler Toolchains",
		version: 1,
		latestVersion: 2,
		createdAt: "2026-11-16T09:00:00Z",
		keyPoints: [
			{
				text: "Baseline single-threaded linker passes dominate debug build time.",
				sourceArticleIds: [SRC_COMPILER_1],
			},
		],
		timeline: [
			{
				date: "2026-11-15",
				text: "Baseline single-threaded linker benchmarks published.",
				sourceArticleIds: [SRC_COMPILER_1],
			},
		],
		whatChanged: [],
		openQuestions: [],
		sources: [
			{
				articleId: SRC_COMPILER_1,
				title: "Incremental Linker Benchmarks",
				url: "https://toolchain.example/incremental-linker",
			},
			{
				articleId: SRC_COMPILER_3,
				title: "",
				url: "https://cache.example/artifact-store",
			},
		],
		versions: compilerVersionsMeta,
	};

	const storageVersionsMeta = [
		{
			version: 1,
			createdAt: "2026-11-17T08:00:00Z",
			articleCount: 2,
		},
	];

	const storageV1: MockThemePage = {
		themeId: THEME_OPENED_ID,
		title: "Storage Engines",
		version: 1,
		latestVersion: 1,
		createdAt: "2026-11-17T08:00:00Z",
		keyPoints: [
			{
				text: "Write-ahead log batching improves fsync throughput on NVMe drives.",
				sourceArticleIds: [SRC_STORAGE_1],
			},
		],
		timeline: [
			{
				date: "2026-11-17",
				text: "Compaction pacing experiments compared leveled and tiered layouts.",
				sourceArticleIds: [SRC_STORAGE_2],
			},
		],
		whatChanged: [],
		openQuestions: [],
		sources: [
			{
				articleId: SRC_STORAGE_1,
				title: "WAL Batching on NVMe",
				url: "https://storage.example/wal-batching",
			},
			{
				articleId: SRC_STORAGE_2,
				title: "Compaction Pacing Study",
				url: "https://systems.example/compaction-pacing",
			},
		],
		versions: storageVersionsMeta,
	};

	return new Map<string, MockThemeRecord>([
		[
			THEME_UNOPENED_ID,
			{
				summary: {
					themeId: THEME_UNOPENED_ID,
					title: "Compiler Toolchains",
					latestVersion: 2,
					articleCount: 3,
					updatedAt: "2026-11-18T10:00:00Z",
					lastOpenedVersion: 0,
				},
				versions: new Map([
					[1, compilerV1],
					[2, compilerV2],
				]),
			},
		],
		[
			THEME_OPENED_ID,
			{
				summary: {
					themeId: THEME_OPENED_ID,
					title: "Storage Engines",
					latestVersion: 1,
					articleCount: 2,
					updatedAt: "2026-11-17T08:00:00Z",
					lastOpenedAt: "2026-11-17T09:00:00Z",
					lastOpenedVersion: 1,
				},
				versions: new Map([[1, storageV1]]),
			},
		],
	]);
}

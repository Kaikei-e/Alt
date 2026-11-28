import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { ThemePageData } from "#lib/connect/reading.js";
import type { ReaderOverlayRouteState } from "#lib/reader/types.js";

let currentPageState = $state<{ reader?: ReaderOverlayRouteState }>({});
let currentPageURL = $state(
	new URL(
		"http://localhost/knowledge/themes/01938e82-7c00-7a7b-9b10-100000000001",
	),
);
let currentParams = $state<Record<string, string>>({
	themeId: "01938e82-7c00-7a7b-9b10-100000000001",
});

const mockPushState = vi.fn(
	(url: string | URL, state: { reader?: ReaderOverlayRouteState }) => {
		currentPageState = state;
		if (url) {
			currentPageURL = new URL(String(url), "http://localhost");
		}
	},
);
const mockReplaceState = vi.fn(
	(url: string | URL, state: { reader?: ReaderOverlayRouteState }) => {
		currentPageState = state;
		if (url) {
			currentPageURL = new URL(String(url), "http://localhost");
		}
	},
);

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
	beforeNavigate: vi.fn(),
	pushState: (url: string | URL, state: { reader?: ReaderOverlayRouteState }) =>
		mockPushState(url, state),
	replaceState: (
		url: string | URL,
		state: { reader?: ReaderOverlayRouteState },
	) => mockReplaceState(url, state),
}));

vi.mock("$app/state", () => ({
	page: {
		get state() {
			return currentPageState;
		},
		get url() {
			return currentPageURL;
		},
		get params() {
			return currentParams;
		},
	},
}));

vi.mock("#lib/connect/reading-transport.js", async (importOriginal) => ({
	...(await importOriginal<
		typeof import("#lib/connect/reading-transport.js")
	>()),
	getReadingTransport: (keepalive = false) =>
		keepalive ? "keepalive-transport" : "client-transport",
}));

vi.mock("#lib/connect/reading.js", async (importOriginal) => {
	const actual =
		await importOriginal<typeof import("#lib/connect/reading.js")>();
	return {
		...actual,
		getTheme: vi.fn(),
		recordThemeOpened: vi.fn().mockResolvedValue(undefined),
		listThemeCurations: vi.fn().mockResolvedValue([]),
		listArticleAnnotations: vi.fn().mockResolvedValue([]),
		curateThemeItem: vi.fn(),
		removeThemeCuration: vi.fn().mockResolvedValue(undefined),
	};
});

vi.mock("#lib/connect/knowledge_trail.js", { spy: true });
vi.mock("#lib/api/client/articles.js", async (importOriginal) => {
	const actual = await importOriginal<Record<string, unknown>>();
	return {
		...actual,
		getFeedContentOnTheFlyClient: vi.fn().mockResolvedValue({
			content: "<p>Source article body.</p>",
			article_id: "01938e82-7c00-7a7b-9b10-200000000002",
		}),
		getArticleSourceURLClient: vi
			.fn()
			.mockResolvedValue({ url: "", title: "" }),
	};
});

import { getItemBranches } from "#lib/connect/knowledge_trail.js";
import {
	AnnotationStance,
	curateThemeItem,
	getTheme,
	listArticleAnnotations,
	listThemeCurations,
	recordThemeOpened,
	removeThemeCuration,
	ThemeCurationKind,
} from "#lib/connect/reading.js";
import ArticleReaderOverlay from "#lib/reader/ArticleReaderOverlay.svelte";
import { resetReaderOverlaySessions } from "#lib/reader/reader-overlay.svelte.js";
import { formatCompactDate } from "#lib/reading/themes-format.js";
import Page from "./+page.svelte";

const THEME_ID = "01938e82-7c00-7a7b-9b10-100000000001";
const SRC_1 = "01938e82-7c00-7a7b-9b10-200000000001";
const SRC_2 = "01938e82-7c00-7a7b-9b10-200000000002";

const VERSION_2_DATA: ThemePageData = {
	themeId: THEME_ID,
	title: "Compiler Toolchains",
	version: 2,
	latestVersion: 2,
	createdAt: new Date("2026-11-18T10:00:00.000Z"),
	keyPoints: [
		{
			text: "Incremental linking cuts rebuild latency.",
			sourceArticleIds: [SRC_2, "missing-source-id"],
		},
	],
	timeline: [
		{
			date: "2026-11-15",
			text: "Initial benchmark published.",
			sourceArticleIds: [SRC_1],
		},
		{
			date: "not-a-date",
			text: "Follow-up notes without valid date.",
			sourceArticleIds: [],
		},
	],
	whatChanged: [
		{
			text: "Parallel symbol resolution landed in main.",
			sourceArticleIds: [SRC_2],
		},
	],
	openQuestions: [
		{
			text: "How do remote caches handle stale object files?",
			sourceArticleIds: [SRC_1],
		},
	],
	sources: [
		{
			articleId: SRC_1,
			title: "",
			url: "https://toolchain.example/benchmarks",
		},
		{
			articleId: SRC_2,
			title: "Linker Parallelism Notes",
			url: "https://runtime.example/linker-notes",
		},
	],
	versions: [
		{
			version: 2,
			createdAt: new Date("2026-11-18T10:00:00.000Z"),
			articleCount: 2,
		},
		{
			version: 1,
			createdAt: new Date("2026-11-16T09:00:00.000Z"),
			articleCount: 1,
		},
	],
};

const VERSION_1_DATA: ThemePageData = {
	themeId: THEME_ID,
	title: "Compiler Toolchains",
	version: 1,
	latestVersion: 2,
	createdAt: new Date("2026-11-16T09:00:00.000Z"),
	keyPoints: [
		{
			text: "Baseline single-threaded link times measured.",
			sourceArticleIds: [SRC_1],
		},
	],
	timeline: [],
	whatChanged: [],
	openQuestions: [],
	sources: [
		{
			articleId: SRC_1,
			title: "Baseline Benchmarks",
			url: "https://toolchain.example/benchmarks",
		},
	],
	versions: VERSION_2_DATA.versions,
};

async function settle(ms = 100) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

describe("/knowledge/themes/[themeId] page", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		resetReaderOverlaySessions();
		currentPageState = {};
		currentPageURL = new URL(`http://localhost/knowledge/themes/${THEME_ID}`);
		currentParams = { themeId: THEME_ID };
		vi.mocked(getItemBranches).mockResolvedValue([]);
		vi.mocked(listThemeCurations).mockResolvedValue([]);
		vi.mocked(listArticleAnnotations).mockResolvedValue([]);
		vi.mocked(removeThemeCuration).mockResolvedValue(undefined);
	});

	it("renders all sections in order, maps markers to sources, omits missing source ids, and opens reader overlay", async () => {
		vi.mocked(getTheme).mockResolvedValueOnce(VERSION_2_DATA);

		render(Page);
		render(ArticleReaderOverlay);
		await settle();

		const backLink = page.getByRole("link", { name: "Themes" });
		await expect.element(backLink).toHaveAttribute("href", "/knowledge");
		await expect
			.element(
				page.getByRole("heading", { level: 1, name: "Compiler Toolchains" }),
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("theme-meta"))
			.toHaveTextContent(
				`Version 2 of 2 · ${formatCompactDate(VERSION_2_DATA.createdAt)} · 2 sources`,
			);
		await expect
			.element(page.getByTestId("theme-older-notice"))
			.not.toBeInTheDocument();

		const headings = page
			.getByRole("heading", { level: 2 })
			.elements()
			.map((el) => el.textContent?.trim());
		expect(headings).toEqual([
			"Key points",
			"Timeline",
			"What changed",
			"Open questions",
			"Sources",
			"Versions",
		]);

		// Key points sentence has [2] (for SRC_2) and NO marker for missing-source-id, placed in its own marker row outside <p>
		const keyPointsSection = page.getByTestId("theme-section-key-points");
		const keyPointMarkerButtons = keyPointsSection
			.getByTestId("theme-marker-row")
			.getByRole("button")
			.elements();
		expect(keyPointMarkerButtons.map((el) => el.textContent?.trim())).toEqual([
			"[2]",
		]);
		expect(keyPointMarkerButtons[0]?.closest("p")).toBeNull();

		// Timeline formats valid YYYY-MM-DD dates compactly and omits unparseable dates + empty marker rows
		const timelineSection = page.getByTestId("theme-section-timeline");
		await expect
			.element(timelineSection.getByText("Nov 15, 2026"))
			.toBeInTheDocument();
		expect(timelineSection.getByText("2026-11-15").elements()).toHaveLength(0);
		expect(timelineSection.getByText("not-a-date").elements()).toHaveLength(0);
		expect(
			timelineSection.getByTestId("theme-marker-row").elements(),
		).toHaveLength(1);

		// Sources: first item falls back to URL host when title is empty
		const sourcesSection = page.getByTestId("theme-section-sources");
		await expect
			.element(sourcesSection.getByText("[1] toolchain.example"))
			.toBeInTheDocument();
		await expect
			.element(sourcesSection.getByText("[2] Linker Parallelism Notes"))
			.toBeInTheDocument();

		// Tapping marker [2] via its accessible name opens reader overlay at source 2
		await keyPointsSection
			.getByRole("button", { name: "Open source 2: Linker Parallelism Notes" })
			.click();
		await expect.element(page.getByTestId("reader-overlay")).toBeVisible();
		expect(currentPageURL.pathname).toBe(`/articles/${SRC_2}`);
		expect(recordThemeOpened).toHaveBeenCalledTimes(1);
		expect(recordThemeOpened).toHaveBeenCalledWith(
			"keepalive-transport",
			THEME_ID,
			2,
		);
	});

	it("omits empty sections, hides Versions when only one version exists, and shows older-version notice", async () => {
		currentPageURL = new URL(
			`http://localhost/knowledge/themes/${THEME_ID}?version=1`,
		);
		const singleVersion = VERSION_1_DATA.versions.slice(1, 2);
		vi.mocked(getTheme).mockResolvedValueOnce({
			...VERSION_1_DATA,
			versions: singleVersion,
		});

		render(Page);
		await settle();

		const notice = page.getByTestId("theme-older-notice");
		await expect.element(notice).toBeInTheDocument();
		await expect
			.element(notice)
			.toHaveTextContent("You are reading an older version.");
		await expect
			.element(notice.getByRole("link", { name: "Latest" }))
			.toHaveAttribute("href", `/knowledge/themes/${THEME_ID}`);

		await expect
			.element(page.getByTestId("theme-section-key-points"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("theme-section-timeline"))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByTestId("theme-section-what-changed"))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByTestId("theme-section-open-questions"))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByTestId("theme-section-versions"))
			.not.toBeInTheDocument();
	});

	it("calls recordThemeOpened once per theme+version and again when version changes", async () => {
		vi.mocked(getTheme)
			.mockResolvedValueOnce(VERSION_2_DATA)
			.mockResolvedValueOnce(VERSION_1_DATA);

		render(Page);
		await settle();

		expect(recordThemeOpened).toHaveBeenCalledTimes(1);
		expect(recordThemeOpened).toHaveBeenNthCalledWith(
			1,
			"keepalive-transport",
			THEME_ID,
			2,
		);

		// Switching query param to ?version=1 reloads and records version 1
		currentPageURL = new URL(
			`http://localhost/knowledge/themes/${THEME_ID}?version=1`,
		);
		await settle();

		expect(getTheme).toHaveBeenNthCalledWith(
			2,
			"client-transport",
			THEME_ID,
			1,
		);
		expect(recordThemeOpened).toHaveBeenCalledTimes(2);
		expect(recordThemeOpened).toHaveBeenNthCalledWith(
			2,
			"keepalive-transport",
			THEME_ID,
			1,
		);
	});

	it("renders not-found state on Connect Code.NotFound with link back to /knowledge", async () => {
		vi.mocked(getTheme).mockRejectedValueOnce(
			new ConnectError("theme not found", Code.NotFound),
		);

		render(Page);
		await settle();

		await expect
			.element(page.getByTestId("theme-not-found"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("This theme does not exist."))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("link", { name: "Themes" }))
			.toHaveAttribute("href", "/knowledge");
		expect(recordThemeOpened).not.toHaveBeenCalled();
	});

	it("renders error state and retries on Retry click", async () => {
		vi.mocked(getTheme)
			.mockRejectedValueOnce(new Error("Temporary error"))
			.mockResolvedValueOnce(VERSION_2_DATA);

		render(Page);
		await settle();

		await expect.element(page.getByTestId("theme-error")).toBeInTheDocument();
		await page.getByRole("button", { name: "Retry" }).click();
		await settle();

		await expect
			.element(
				page.getByRole("heading", { level: 1, name: "Compiler Toolchains" }),
			)
			.toBeInTheDocument();
	});

	it("supports pinning key points, adding user questions, resolving open questions, and rendering source takeaways and promoted insights", async () => {
		vi.mocked(getTheme).mockResolvedValueOnce(VERSION_2_DATA);
		vi.mocked(listThemeCurations).mockResolvedValueOnce([
			{
				curationId: "cur-augur-1",
				themeId: THEME_ID,
				kind: ThemeCurationKind.PROMOTE_AUGUR_INSIGHT,
				targetHash: "",
				targetText: "Why did rebuild latency drop?",
				userNote: "Parallel symbol tables removed lock contention.",
				augurConversationId: "conv-77",
				createdAt: new Date("2026-11-19T12:00:00Z"),
			},
		]);
		vi.mocked(listArticleAnnotations).mockResolvedValueOnce([
			{
				annotationId: "ann-1",
				articleId: SRC_2,
				stance: AnnotationStance.INSIGHT,
				excerpt: "Parallel symbol merging scales linearly up to 16 cores.",
				note: "Check our CI runner core count.",
				createdAt: new Date("2026-11-19T10:00:00Z"),
			},
		]);
		vi.mocked(curateThemeItem).mockResolvedValueOnce({
			curationId: "cur-pin-1",
			themeId: THEME_ID,
			kind: ThemeCurationKind.PIN_KEY_POINT,
			targetHash: "hash-kp-1",
			targetText: "Incremental linking cuts rebuild latency.",
			userNote: "",
			augurConversationId: "",
			createdAt: new Date("2026-11-20T00:00:00Z"),
		});
		vi.mocked(curateThemeItem).mockResolvedValueOnce({
			curationId: "cur-q-1",
			themeId: THEME_ID,
			kind: ThemeCurationKind.ADD_QUESTION,
			targetHash: "",
			targetText: "Does remote caching help incremental Rust builds?",
			userNote: "",
			augurConversationId: "",
			createdAt: new Date("2026-11-20T01:00:00Z"),
		});

		render(Page);
		await settle();

		// Promoted Augur insight & user source takeaways render
		await expect
			.element(page.getByTestId("theme-section-promoted-insights"))
			.toBeInTheDocument();
		await expect
			.element(
				page.getByText("Parallel symbol tables removed lock contention."),
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("link", { name: "Open thread" }))
			.toHaveAttribute(
				"href",
				`/augur/conv-77?themeId=${THEME_ID}&themeTitle=Compiler+Toolchains`,
			);
		await expect
			.element(page.getByTestId("theme-section-takeaways"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Check our CI runner core count."))
			.toBeInTheDocument();

		// Open question includes Ask Augur link carrying themeId
		const askAugurLink = page
			.getByTestId("theme-section-open-questions")
			.getByRole("link", { name: "Ask Augur" });
		await expect.element(askAugurLink).toBeInTheDocument();
		expect(askAugurLink.element().getAttribute("href")).toContain(
			`themeId=${THEME_ID}`,
		);

		// Pin the key point
		await page
			.getByTestId("theme-keypoint-actions")
			.getByRole("button", { name: "Pin" })
			.click();
		await settle();
		expect(curateThemeItem).toHaveBeenCalledWith(
			"client-transport",
			expect.objectContaining({
				themeId: THEME_ID,
				kind: ThemeCurationKind.PIN_KEY_POINT,
				targetText: "Incremental linking cuts rebuild latency.",
			}),
		);
		await expect
			.element(
				page
					.getByTestId("theme-keypoint-actions")
					.getByRole("button", { name: "Pinned" }),
			)
			.toBeInTheDocument();

		// Add a user question via composer
		await page.getByTestId("theme-add-curation-toggle").click();
		await page
			.getByPlaceholder("Add an open question to explore…")
			.fill("Does remote caching help incremental Rust builds?");
		await page.getByRole("button", { name: "Save", exact: true }).click();
		await settle();

		await expect
			.element(
				page.getByText("Does remote caching help incremental Rust builds?"),
			)
			.toBeInTheDocument();
	});
});

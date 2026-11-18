import { create, toJson } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { createConnectTransport } from "@connectrpc/connect-web";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	getTheme,
	listThemes,
	recordThemeOpened,
} from "#lib/connect/reading.js";
import {
	GetThemeRequestSchema,
	GetThemeResponseSchema,
	ListThemesRequestSchema,
	ListThemesResponseSchema,
	RecordThemeOpenedRequestSchema,
} from "#lib/gen/alt/reading/v1/reading_pb.js";

const UUID_PATTERN =
	/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

describe("Theme Pages ReadingService Contract", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	describe("Proto JSON wire shapes", () => {
		it("serializes ListThemesRequest and ListThemesResponse", () => {
			const req = create(ListThemesRequestSchema, {});
			expect(toJson(ListThemesRequestSchema, req)).toEqual({});

			const res = create(ListThemesResponseSchema, {
				themes: [
					{
						themeId: "01938e82-7c00-7a7b-9b10-100000000001",
						title: "Compiler Toolchains",
						latestVersion: 2,
						articleCount: 3,
						updatedAt: timestampFromDate(new Date("2026-11-18T10:00:00.000Z")),
						lastOpenedVersion: 0,
					},
				],
			});
			expect(toJson(ListThemesResponseSchema, res)).toEqual({
				themes: [
					{
						themeId: "01938e82-7c00-7a7b-9b10-100000000001",
						title: "Compiler Toolchains",
						latestVersion: 2,
						articleCount: 3,
						updatedAt: "2026-11-18T10:00:00Z",
					},
				],
			});
		});

		it("serializes GetThemeRequest with and without optional version", () => {
			const latestReq = create(GetThemeRequestSchema, {
				themeId: "01938e82-7c00-7a7b-9b10-100000000001",
			});
			expect(toJson(GetThemeRequestSchema, latestReq)).toEqual({
				themeId: "01938e82-7c00-7a7b-9b10-100000000001",
			});

			const pinnedReq = create(GetThemeRequestSchema, {
				themeId: "01938e82-7c00-7a7b-9b10-100000000001",
				version: 1,
			});
			expect(toJson(GetThemeRequestSchema, pinnedReq)).toEqual({
				themeId: "01938e82-7c00-7a7b-9b10-100000000001",
				version: 1,
			});

			const res = create(GetThemeResponseSchema, {
				themeId: "01938e82-7c00-7a7b-9b10-100000000001",
				title: "Compiler Toolchains",
				version: 2,
				latestVersion: 2,
				createdAt: timestampFromDate(new Date("2026-11-18T10:00:00.000Z")),
				keyPoints: [
					{
						text: "Incremental linking reduces build times.",
						sourceArticleIds: ["art-1"],
					},
				],
				timeline: [
					{
						date: "2026-11-18",
						text: "Linker release candidate published.",
						sourceArticleIds: ["art-1"],
					},
				],
				whatChanged: [
					{
						text: "Added parallel symbol resolution.",
						sourceArticleIds: ["art-2"],
					},
				],
				openQuestions: [
					{
						text: "How does remote caching invalidate stale archives?",
						sourceArticleIds: ["art-2"],
					},
				],
				sources: [
					{
						articleId: "art-1",
						title: "Linker Notes",
						url: "https://toolchain.example/linker",
					},
				],
				versions: [
					{
						version: 2,
						createdAt: timestampFromDate(new Date("2026-11-18T10:00:00.000Z")),
						articleCount: 3,
					},
				],
			});
			expect(toJson(GetThemeResponseSchema, res)).toMatchObject({
				themeId: "01938e82-7c00-7a7b-9b10-100000000001",
				version: 2,
				latestVersion: 2,
				createdAt: "2026-11-18T10:00:00Z",
				keyPoints: [
					{
						text: "Incremental linking reduces build times.",
						sourceArticleIds: ["art-1"],
					},
				],
			});
		});

		it("serializes RecordThemeOpenedRequest with exact wire field names", () => {
			const req = create(RecordThemeOpenedRequestSchema, {
				themeId: "01938e82-7c00-7a7b-9b10-100000000001",
				version: 2,
				clientEventId: "01938e82-7c00-7a7b-9b10-000000000099",
				openedAt: timestampFromDate(new Date("2026-11-18T11:00:00.000Z")),
			});
			expect(toJson(RecordThemeOpenedRequestSchema, req)).toEqual({
				themeId: "01938e82-7c00-7a7b-9b10-100000000001",
				version: 2,
				clientEventId: "01938e82-7c00-7a7b-9b10-000000000099",
				openedAt: "2026-11-18T11:00:00Z",
			});
		});
	});

	describe("listThemes wrapper", () => {
		it("calls ReadingService/ListThemes and returns plain TS summaries with Date timestamps", async () => {
			let capturedUrl = "";
			const stubFetch = vi.fn(async (input: RequestInfo | URL) => {
				capturedUrl = String(input);
				return new Response(
					JSON.stringify({
						themes: [
							{
								themeId: "01938e82-7c00-7a7b-9b10-100000000001",
								title: "Compiler Toolchains",
								latestVersion: 2,
								articleCount: 3,
								updatedAt: "2026-11-18T10:00:00Z",
								lastOpenedVersion: 0,
							},
							{
								themeId: "01938e82-7c00-7a7b-9b10-100000000002",
								title: "Storage Engines",
								latestVersion: 1,
								articleCount: 2,
								updatedAt: "2026-11-17T08:00:00Z",
								lastOpenedAt: "2026-11-17T09:00:00Z",
								lastOpenedVersion: 1,
							},
						],
					}),
					{ status: 200, headers: { "Content-Type": "application/json" } },
				);
			});
			const transport = createConnectTransport({
				baseUrl: "http://127.0.0.1/api/v2",
				fetch: stubFetch,
			});

			const result = await listThemes(transport);

			expect(capturedUrl).toBe(
				"http://127.0.0.1/api/v2/alt.reading.v1.ReadingService/ListThemes",
			);
			expect(result).toHaveLength(2);
			expect(result[0]).toEqual({
				themeId: "01938e82-7c00-7a7b-9b10-100000000001",
				title: "Compiler Toolchains",
				latestVersion: 2,
				articleCount: 3,
				updatedAt: new Date("2026-11-18T10:00:00.000Z"),
				lastOpenedAt: null,
				lastOpenedVersion: 0,
			});
			expect(result[1]?.lastOpenedAt).toEqual(
				new Date("2026-11-17T09:00:00.000Z"),
			);
			expect("$typeName" in (result[0] ?? {})).toBe(false);
		});
	});

	describe("getTheme wrapper", () => {
		it("omits version when undefined and sends version when specified", async () => {
			const capturedBodies: string[] = [];
			const stubFetch = vi.fn(
				async (_input: RequestInfo | URL, init?: RequestInit) => {
					if (typeof init?.body === "string") {
						capturedBodies.push(init.body);
					} else if (init?.body instanceof Uint8Array) {
						capturedBodies.push(new TextDecoder().decode(init.body));
					}
					return new Response(
						JSON.stringify({
							themeId: "01938e82-7c00-7a7b-9b10-100000000001",
							title: "Compiler Toolchains",
							version: 1,
							latestVersion: 2,
							createdAt: "2026-11-16T09:00:00Z",
							keyPoints: [{ text: "Point A", sourceArticleIds: ["art-1"] }],
							timeline: [
								{
									date: "2026-11-15",
									text: "Event A",
									sourceArticleIds: ["art-1"],
								},
							],
							whatChanged: [],
							openQuestions: [],
							sources: [
								{
									articleId: "art-1",
									title: "Compiler Notes",
									url: "https://toolchain.example/notes",
								},
							],
							versions: [
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
							],
						}),
						{ status: 200, headers: { "Content-Type": "application/json" } },
					);
				},
			);
			const transport = createConnectTransport({
				baseUrl: "http://127.0.0.1/api/v2",
				fetch: stubFetch,
			});

			await getTheme(
				"01938e82-7c00-7a7b-9b10-100000000001",
				undefined,
				transport,
			);
			const page = await getTheme(
				"01938e82-7c00-7a7b-9b10-100000000001",
				1,
				transport,
			);

			expect(JSON.parse(capturedBodies[0] ?? "{}")).toEqual({
				themeId: "01938e82-7c00-7a7b-9b10-100000000001",
			});
			expect(JSON.parse(capturedBodies[1] ?? "{}")).toEqual({
				themeId: "01938e82-7c00-7a7b-9b10-100000000001",
				version: 1,
			});

			expect(page.createdAt).toEqual(new Date("2026-11-16T09:00:00.000Z"));
			expect(page.versions[0]?.createdAt).toEqual(
				new Date("2026-11-18T10:00:00.000Z"),
			);
			expect("$typeName" in page).toBe(false);
			expect("$typeName" in (page.keyPoints[0] ?? {})).toBe(false);
			expect("$typeName" in (page.sources[0] ?? {})).toBe(false);
		});
	});

	describe("recordThemeOpened wrapper", () => {
		it("mints a UUID clientEventId and openedAt timestamp on the wire", async () => {
			let capturedUrl = "";
			let capturedBody = "";
			const stubFetch = vi.fn(
				async (input: RequestInfo | URL, init?: RequestInit) => {
					capturedUrl = String(input);
					if (typeof init?.body === "string") {
						capturedBody = init.body;
					} else if (init?.body instanceof Uint8Array) {
						capturedBody = new TextDecoder().decode(init.body);
					}
					return new Response(JSON.stringify({}), {
						status: 200,
						headers: { "Content-Type": "application/json" },
					});
				},
			);
			const transport = createConnectTransport({
				baseUrl: "http://127.0.0.1/api/v2",
				fetch: stubFetch,
			});

			await recordThemeOpened("01938e82-7c00-7a7b-9b10-100000000001", 2, {
				transport,
			});

			expect(capturedUrl).toBe(
				"http://127.0.0.1/api/v2/alt.reading.v1.ReadingService/RecordThemeOpened",
			);
			const parsed = JSON.parse(capturedBody) as Record<string, unknown>;
			expect(parsed.themeId).toBe("01938e82-7c00-7a7b-9b10-100000000001");
			expect(parsed.version).toBe(2);
			expect(String(parsed.clientEventId)).toMatch(UUID_PATTERN);
			expect(typeof parsed.openedAt).toBe("string");
			expect(Number.isNaN(Date.parse(String(parsed.openedAt)))).toBe(false);
		});

		it("swallows failures after a console.warn without leaking any identifiers", async () => {
			const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
			const secretThemeId = "01938e82-7c00-7a7b-9b10-999999999999";
			const secretEventId = "01938e82-7c00-7a7b-9b10-888888888888";
			const stubFetch = vi.fn(async () => {
				return new Response(
					JSON.stringify({
						code: "unavailable",
						message: `downstream failure for ${secretThemeId}`,
					}),
					{ status: 503, headers: { "Content-Type": "application/json" } },
				);
			});
			const transport = createConnectTransport({
				baseUrl: "http://127.0.0.1/api/v2",
				fetch: stubFetch,
			});

			await expect(
				recordThemeOpened(secretThemeId, 2, {
					transport,
					clientEventId: secretEventId,
				}),
			).resolves.toBeUndefined();

			expect(warnSpy).toHaveBeenCalledTimes(1);
			const logged = warnSpy.mock.calls.flat().join(" ");
			expect(logged).not.toContain(secretThemeId);
			expect(logged).not.toContain(secretEventId);
		});
	});
});

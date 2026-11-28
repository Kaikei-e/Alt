import net from "node:net";
import { describe, expect, it } from "vitest";
import { createBackendServer } from "./backend";

describe("E2E mock backend server", () => {
	it("waits for a chunked POST body to complete before sending response headers on a keep-alive socket", async () => {
		const server = createBackendServer();
		let headersSentBeforeBody: boolean | undefined;

		await new Promise<void>((resolve) => {
			server.listen(0, "127.0.0.1", () => resolve());
		});
		const address = server.address();
		if (!address || typeof address === "string") {
			server.close();
			throw new Error("Expected TCP server address");
		}

		const socket = new net.Socket();
		try {
			const secondResponseStatusLine = await new Promise<string>(
				(resolve, reject) => {
					let raw = "";
					let sentSecondRequest = false;
					let settled = false;

					socket.setTimeout(2000, () => {
						if (!settled) {
							settled = true;
							reject(new Error("Socket timed out after 2000ms"));
						}
					});

					socket.on("close", () => {
						if (!settled) {
							settled = true;
							reject(
								new Error("Socket closed before receiving second response"),
							);
						}
					});

					socket.on("error", (err) => {
						if (!settled) {
							settled = true;
							reject(err);
						}
					});

					server.once("request", (_req, res) => {
						setImmediate(() => {
							headersSentBeforeBody = res.headersSent;
							socket.write("2\r\n{}\r\n0\r\n\r\n");
						});
					});

					socket.on("connect", () => {
						// Send only the chunked POST headers first; the body is written
						// after the server emits 'request' and a setImmediate turn elapses.
						socket.write(
							"POST /alt.knowledge_home.v1.KnowledgeHomeService/TrackHomeAction HTTP/1.1\r\n" +
								`Host: 127.0.0.1:${address.port}\r\n` +
								"Content-Type: application/json\r\n" +
								"Transfer-Encoding: chunked\r\n" +
								"Connection: keep-alive\r\n\r\n",
						);
					});

					socket.on("data", (chunk) => {
						raw += chunk.toString("utf-8");
						const headerEnd = raw.indexOf("\r\n\r\n");
						if (
							!sentSecondRequest &&
							headerEnd !== -1 &&
							raw.slice(headerEnd + 4).includes("\r\n0\r\n\r\n")
						) {
							sentSecondRequest = true;
							raw = "";
							socket.write(
								"POST /alt.articles.v2.ArticleService/FetchRandomFeed HTTP/1.1\r\n" +
									`Host: 127.0.0.1:${address.port}\r\n` +
									"Content-Type: application/json\r\n" +
									"Content-Length: 2\r\n" +
									"Connection: close\r\n\r\n{}",
							);
							return;
						}
						if (sentSecondRequest && raw.includes("\r\n")) {
							const firstLine = raw.split("\r\n")[0] ?? "";
							settled = true;
							resolve(firstLine);
						}
					});

					socket.connect(address.port, "127.0.0.1");
				},
			);

			expect(headersSentBeforeBody).toBe(false);
			expect(secondResponseStatusLine).toBe("HTTP/1.1 200 OK");
		} finally {
			socket.destroy();
			server.closeAllConnections();
			await new Promise<void>((resolve) => server.close(() => resolve()));
		}
	});

	it("handles QueueArticle -> GetReadTodayQueue -> UnqueueArticle statefully and enforces cap 7", async () => {
		const server = createBackendServer();
		await new Promise<void>((resolve) => {
			server.listen(0, "127.0.0.1", () => resolve());
		});
		const address = server.address();
		if (!address || typeof address === "string") {
			server.close();
			throw new Error("Expected TCP server address");
		}
		const baseUrl = `http://127.0.0.1:${address.port}`;

		try {
			const postRpc = async (method: string, body: Record<string, unknown>) => {
				const res = await fetch(
					`${baseUrl}/alt.reading.v1.ReadingService/${method}`,
					{
						method: "POST",
						headers: { "Content-Type": "application/json" },
						body: JSON.stringify(body),
					},
				);
				return res.json() as Promise<Record<string, unknown>>;
			};

			const testUuid = (n: number) =>
				`01938e82-7c00-7a7b-9b10-${String(n).padStart(12, "0")}`;

			const initial = await postRpc("GetReadTodayQueue", {});
			expect(initial).toMatchObject({ items: [], cap: 7, readCountSince: 0 });

			for (let i = 1; i <= 7; i++) {
				const q = await postRpc("QueueArticle", {
					articleId: testUuid(i),
				});
				expect(q.outcome).toBe("QUEUE_ARTICLE_OUTCOME_QUEUED");
				expect(q.items).toHaveLength(i);
			}

			const full = await postRpc("QueueArticle", { articleId: testUuid(8) });
			expect(full.outcome).toBe("QUEUE_ARTICLE_OUTCOME_QUEUE_FULL");
			expect(full.items).toHaveLength(7);

			await postRpc("UnqueueArticle", { articleId: testUuid(1) });
			const afterRemove = await postRpc("GetReadTodayQueue", {});
			expect(afterRemove.items).toHaveLength(6);
		} finally {
			server.closeAllConnections();
			await new Promise<void>((resolve) => server.close(() => resolve()));
		}
	});

	it("rejects non-UUID or empty articleId in QueueArticle and UnqueueArticle with HTTP 400 invalid_argument", async () => {
		const server = createBackendServer();
		await new Promise<void>((resolve) => {
			server.listen(0, "127.0.0.1", () => resolve());
		});
		const address = server.address();
		if (!address || typeof address === "string") {
			server.close();
			throw new Error("Expected TCP server address");
		}
		const baseUrl = `http://127.0.0.1:${address.port}`;

		try {
			for (const method of ["QueueArticle", "UnqueueArticle"]) {
				for (const badId of ["", "not-a-uuid", "deck-art-1"]) {
					const res = await fetch(
						`${baseUrl}/alt.reading.v1.ReadingService/${method}`,
						{
							method: "POST",
							headers: { "Content-Type": "application/json" },
							body: JSON.stringify({ articleId: badId }),
						},
					);
					expect(res.status).toBe(400);
					const json = (await res.json()) as { code?: string };
					expect(json.code).toBe("invalid_argument");
				}
			}
		} finally {
			server.closeAllConnections();
			await new Promise<void>((resolve) => server.close(() => resolve()));
		}
	});

	it("returns readArticleIds from RecordReadingEvents for qualifying sessionEnded and drops them from the queue", async () => {
		const server = createBackendServer();
		await new Promise<void>((resolve) => {
			server.listen(0, "127.0.0.1", () => resolve());
		});
		const address = server.address();
		if (!address || typeof address === "string") {
			server.close();
			throw new Error("Expected TCP server address");
		}
		const baseUrl = `http://127.0.0.1:${address.port}`;
		const articleId = "01938e82-7c00-7a7b-9b10-000000000099";

		try {
			await fetch(`${baseUrl}/alt.reading.v1.ReadingService/QueueArticle`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ articleId }),
			});

			const res = await fetch(
				`${baseUrl}/alt.reading.v1.ReadingService/RecordReadingEvents`,
				{
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({
						events: [
							{
								clientEventId: "01938e82-7c00-7a7b-9b10-100000000001",
								occurredAt: new Date().toISOString(),
								sessionEnded: {
									articleId,
									surface: "READING_SURFACE_OTHER",
									activeDwellMs: "2000",
									maxScrollRatio: 1,
									bodyLengthChars: 500,
									reachedEnd: true,
								},
							},
						],
					}),
				},
			);
			expect(res.status).toBe(200);
			const recorded = (await res.json()) as { readArticleIds?: string[] };
			expect(recorded.readArticleIds).toEqual([articleId]);

			const queueRes = await fetch(
				`${baseUrl}/alt.reading.v1.ReadingService/GetReadTodayQueue`,
				{
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({}),
				},
			);
			const queueJson = (await queueRes.json()) as {
				items: unknown[];
				readCountSince: number;
			};
			expect(queueJson.items).toHaveLength(0);
			expect(queueJson.readCountSince).toBeGreaterThanOrEqual(1);
		} finally {
			server.closeAllConnections();
			await new Promise<void>((resolve) => server.close(() => resolve()));
		}
	});

	it("handles ListThemes, GetTheme (404 not_found / 400 invalid_argument), and RecordThemeOpened statefully", async () => {
		const server = createBackendServer();
		await new Promise<void>((resolve) => {
			server.listen(0, "127.0.0.1", () => resolve());
		});
		const address = server.address();
		if (!address || typeof address === "string") {
			server.close();
			throw new Error("Expected TCP server address");
		}
		const baseUrl = `http://127.0.0.1:${address.port}`;

		try {
			const postThemeRpc = async (
				method: string,
				body: Record<string, unknown>,
			) => {
				const res = await fetch(
					`${baseUrl}/alt.reading.v1.ReadingService/${method}`,
					{
						method: "POST",
						headers: { "Content-Type": "application/json" },
						body: JSON.stringify(body),
					},
				);
				return {
					status: res.status,
					json: (await res.json()) as Record<string, unknown>,
				};
			};

			const listRes = await postThemeRpc("ListThemes", {});
			expect(listRes.status).toBe(200);
			const themes = listRes.json.themes as Array<{
				themeId: string;
				latestVersion: number;
				lastOpenedVersion: number;
			}>;
			expect(themes).toHaveLength(2);
			const unopened = themes.find((t) => t.lastOpenedVersion === 0);
			const opened = themes.find((t) => t.lastOpenedVersion > 0);
			expect(unopened).toBeDefined();
			expect(unopened?.latestVersion).toBe(2);
			expect(opened).toBeDefined();
			expect(opened?.lastOpenedVersion).toBe(opened?.latestVersion);

			// Non-UUID themeId -> 400 invalid_argument
			const badIdRes = await postThemeRpc("GetTheme", {
				themeId: "not-a-uuid",
			});
			expect(badIdRes.status).toBe(400);
			expect(badIdRes.json.code).toBe("invalid_argument");

			// Unknown UUID -> 404 not_found
			const missingIdRes = await postThemeRpc("GetTheme", {
				themeId: "01938e82-7c00-7a7b-9b10-999999999999",
			});
			expect(missingIdRes.status).toBe(404);
			expect(missingIdRes.json.code).toBe("not_found");

			// Unknown version -> 404 not_found
			const missingVersionRes = await postThemeRpc("GetTheme", {
				themeId: unopened?.themeId,
				version: 99,
			});
			expect(missingVersionRes.status).toBe(404);
			expect(missingVersionRes.json.code).toBe("not_found");

			// Version 2 has whatChanged and openQuestions; Version 1 has neither
			const v2Res = await postThemeRpc("GetTheme", {
				themeId: unopened?.themeId,
			});
			expect(v2Res.status).toBe(200);
			expect(v2Res.json.version).toBe(2);
			expect((v2Res.json.whatChanged as unknown[]).length).toBeGreaterThan(0);
			expect((v2Res.json.openQuestions as unknown[]).length).toBeGreaterThan(0);

			const v1Res = await postThemeRpc("GetTheme", {
				themeId: unopened?.themeId,
				version: 1,
			});
			expect(v1Res.status).toBe(200);
			expect(v1Res.json.version).toBe(1);
			expect(v1Res.json.whatChanged).toEqual([]);
			expect(v1Res.json.openQuestions).toEqual([]);

			// RecordThemeOpened records the call and updates lastOpenedVersion
			const recordRes = await postThemeRpc("RecordThemeOpened", {
				themeId: unopened?.themeId,
				version: 2,
				clientEventId: "01938e82-7c00-7a7b-9b10-000000000099",
				openedAt: new Date().toISOString(),
			});
			expect(recordRes.status).toBe(200);

			const recordedCallsRes = await fetch(`${baseUrl}/__mock/themes/opened`);
			const recordedCallsJson = (await recordedCallsRes.json()) as {
				calls: Array<{ themeId: string; version: number }>;
			};
			expect(recordedCallsJson.calls).toContainEqual(
				expect.objectContaining({
					themeId: unopened?.themeId,
					version: 2,
				}),
			);

			const afterList = await postThemeRpc("ListThemes", {});
			const updatedTheme = (
				afterList.json.themes as Array<{
					themeId: string;
					lastOpenedVersion: number;
				}>
			).find((t) => t.themeId === unopened?.themeId);
			expect(updatedTheme?.lastOpenedVersion).toBe(2);
		} finally {
			server.closeAllConnections();
			await new Promise<void>((resolve) => server.close(() => resolve()));
		}
	});

	it("handles SaveArticleAnnotation/List/Delete and CurateThemeItem/List/Remove statefully", async () => {
		const server = createBackendServer();
		await new Promise<void>((resolve) => {
			server.listen(0, "127.0.0.1", () => resolve());
		});
		const address = server.address();
		if (!address || typeof address === "string") {
			server.close();
			throw new Error("Expected TCP server address");
		}
		const baseUrl = `http://127.0.0.1:${address.port}`;
		const articleId = "01938e82-7c00-7a7b-9b10-000000000042";
		const themeId = "01938e82-7c00-7a7b-9b10-100000000001";

		try {
			const postRpc = async (method: string, body: Record<string, unknown>) => {
				const res = await fetch(
					`${baseUrl}/alt.reading.v1.ReadingService/${method}`,
					{
						method: "POST",
						headers: { "Content-Type": "application/json" },
						body: JSON.stringify(body),
					},
				);
				return {
					status: res.status,
					json: (await res.json()) as Record<string, unknown>,
				};
			};

			const savedAnn = await postRpc("SaveArticleAnnotation", {
				articleId,
				stance: "ANNOTATION_STANCE_TAKEAWAY",
				note: "Key takeaway from test",
			});
			expect(savedAnn.status).toBe(200);
			const annId = (savedAnn.json.annotation as { annotationId: string })
				.annotationId;

			const listedAnn = await postRpc("ListArticleAnnotations", {
				articleIds: [articleId],
			});
			expect(listedAnn.status).toBe(200);
			expect(listedAnn.json.annotations).toHaveLength(1);

			await postRpc("DeleteArticleAnnotation", { annotationId: annId });
			const listedAnnAfter = await postRpc("ListArticleAnnotations", {
				articleIds: [articleId],
			});
			expect(listedAnnAfter.json.annotations).toHaveLength(0);

			const savedCur = await postRpc("CurateThemeItem", {
				themeId,
				kind: "THEME_CURATION_KIND_PIN_KEY_POINT",
				targetHash: "abc123",
				targetText: "Important point",
			});
			expect(savedCur.status).toBe(200);
			const curId = (savedCur.json.curation as { curationId: string })
				.curationId;

			const listedCur = await postRpc("ListThemeCurations", { themeId });
			expect(listedCur.status).toBe(200);
			expect(listedCur.json.curations).toHaveLength(1);

			await postRpc("RemoveThemeCuration", { curationId: curId });
			const listedCurAfter = await postRpc("ListThemeCurations", { themeId });
			expect(listedCurAfter.json.curations).toHaveLength(0);
		} finally {
			server.closeAllConnections();
			await new Promise<void>((resolve) => server.close(() => resolve()));
		}
	});
});

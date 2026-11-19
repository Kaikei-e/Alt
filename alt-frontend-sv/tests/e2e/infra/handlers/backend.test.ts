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
});

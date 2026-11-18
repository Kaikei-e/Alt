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

			const initial = await postRpc("GetReadTodayQueue", {});
			expect(initial).toMatchObject({ items: [], cap: 7, readCountSince: 0 });

			for (let i = 1; i <= 7; i++) {
				const q = await postRpc("QueueArticle", {
					articleId: i === 1 ? "article-1" : `art-${i}`,
				});
				expect(q.outcome).toBe("QUEUE_ARTICLE_OUTCOME_QUEUED");
				expect(q.items).toHaveLength(i);
			}

			const full = await postRpc("QueueArticle", { articleId: "art-8" });
			expect(full.outcome).toBe("QUEUE_ARTICLE_OUTCOME_QUEUE_FULL");
			expect(full.items).toHaveLength(7);

			await postRpc("UnqueueArticle", { articleId: "article-1" });
			const afterRemove = await postRpc("GetReadTodayQueue", {});
			expect(afterRemove.items).toHaveLength(6);
		} finally {
			server.closeAllConnections();
			await new Promise<void>((resolve) => server.close(() => resolve()));
		}
	});
});

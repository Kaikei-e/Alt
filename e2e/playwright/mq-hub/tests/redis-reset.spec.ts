import * as fs from "node:fs";
import * as net from "node:net";
import * as os from "node:os";
import * as path from "node:path";
import { test, expect } from "@playwright/test";
import { CanonicalStream } from "../src/env.js";
import {
	canResetRedis,
	resetCanonicalStreams,
	assertCanonicalStreamsReset,
	CANONICAL_RESET_KEYS,
	encodeRespCommand,
} from "../src/redis.js";

test.describe("Redis reset safety guard and protocol", () => {
	test("refuses reset when explicit opt-in is not granted", () => {
		const check = canResetRedis({
			allowReset: false,
			redisUrl: "redis://redis-streams:6379",
		});
		expect(check.allowed).toBe(false);
		expect(check.reason).toBe("opt_in_required");
	});

	test("refuses reset when redisUrl is missing", () => {
		const check = canResetRedis({
			allowReset: true,
			redisUrl: "",
		});
		expect(check.allowed).toBe(false);
		expect(check.reason).toBe("redis_url_missing");
	});

	test("refuses reset when pointing to protected localhost / user running stack", () => {
		for (const host of ["localhost", "127.0.0.1", "0.0.0.0"]) {
			const check = canResetRedis({
				allowReset: true,
				redisUrl: `redis://${host}:6379`,
				allowLocalhostForTesting: false,
			});
			expect(check.allowed).toBe(false);
			expect(check.reason).toBe("localhost_refused");
		}
	});

	test("strictly rejects non-canonical stream keys and prevents arbitrary deletion", () => {
		expect(() => {
			canResetRedis({
				allowReset: true,
				redisUrl: "redis://redis-streams:6379",
				streamsToReset: ["arbitrary:user:data"],
			});
		}).toThrow("Refusing to reset non-canonical stream key: arbitrary:user:data");

		expect(() => {
			canResetRedis({
				allowReset: true,
				redisUrl: "redis://redis-streams:6379",
				streamsToReset: [CanonicalStream.articles, "production:backup"],
			});
		}).toThrow("Refusing to reset non-canonical stream key: production:backup");
	});

	test("allows reset when all safety conditions are met", () => {
		const check = canResetRedis({
			allowReset: true,
			redisUrl: "redis://redis-streams:6379",
			streamsToReset: CANONICAL_RESET_KEYS,
		});
		expect(check.allowed).toBe(true);
	});

	test("resetCanonicalStreams no-op when opt-in is missing without network activity", async () => {
		const result = await resetCanonicalStreams({
			allowReset: false,
			redisUrl: "redis://nonexistent-host-that-would-fail:6379",
		});
		expect(result.executed).toBe(false);
		expect(result.reason).toBe("opt_in_required");
	});

	test("resetCanonicalStreams refuses localhost without test override", async () => {
		const result = await resetCanonicalStreams({
			allowReset: true,
			redisUrl: "redis://127.0.0.1:6379",
			allowLocalhostForTesting: false,
		});
		expect(result.executed).toBe(false);
		expect(result.reason).toBe("localhost_refused");
	});

	test("RESP wire protocol reset executes correctly against offline mock server", async () => {
		const receivedCommands: string[][] = [];

		// Create in-process mock Redis TCP server
		const server = net.createServer((socket) => {
			let buffer = Buffer.alloc(0);

			socket.on("data", (chunk) => {
				buffer = Buffer.concat([buffer, chunk]);

				// Minimal RESP parser for array of bulk strings (*N\r\n$M\r\n...)
				while (buffer.length > 0) {
					if (buffer[0] !== 42) {
						// Not '*'
						break;
					}
					const crlf1 = buffer.indexOf("\r\n");
					if (crlf1 === -1) break;

					const numArgs = Number.parseInt(buffer.subarray(1, crlf1).toString("utf8"), 10);
					let offset = crlf1 + 2;
					const args: string[] = [];
					let parsedFull = true;

					for (let i = 0; i < numArgs; i++) {
						if (offset >= buffer.length || buffer[offset] !== 36) {
							// '$'
							parsedFull = false;
							break;
						}
						const crlfArg = buffer.indexOf("\r\n", offset);
						if (crlfArg === -1) {
							parsedFull = false;
							break;
						}
						const argLen = Number.parseInt(buffer.subarray(offset + 1, crlfArg).toString("utf8"), 10);
						const valStart = crlfArg + 2;
						const valEnd = valStart + argLen;
						if (valEnd + 2 > buffer.length) {
							parsedFull = false;
							break;
						}
						args.push(buffer.subarray(valStart, valEnd).toString("utf8"));
						offset = valEnd + 2;
					}

					if (!parsedFull) break;

					buffer = buffer.subarray(offset);
					receivedCommands.push(args);

					// Mock Redis responses
					const cmd = args[0]?.toUpperCase();
					if (cmd === "AUTH") {
						socket.write("+OK\r\n");
					} else if (cmd === "DEL") {
						// Reply with number of keys deleted (5)
						socket.write(`:${args.length - 1}\r\n`);
					} else if (cmd === "QUIT") {
						socket.write("+OK\r\n");
					}
				}
			});
		});

		await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
		const addr = server.address() as net.AddressInfo;
		const mockPort = addr.port;

		try {
			const result = await resetCanonicalStreams({
				allowReset: true,
				allowLocalhostForTesting: true,
				redisUrl: `redis://127.0.0.1:${mockPort}`,
				password: "test-staging-secret",
				streamsToReset: CANONICAL_RESET_KEYS,
			});

			expect(result.executed).toBe(true);
			expect(result.deletedCount).toBe(5);

			// Verify wire commands sequence
			expect(receivedCommands).toHaveLength(3);

			// 1. AUTH
			expect(receivedCommands[0]).toEqual(["AUTH", "test-staging-secret"]);

			// 2. DEL only allowed canonical keys
			expect(receivedCommands[1]?.[0]).toBe("DEL");
			expect(receivedCommands[1]?.slice(1)).toEqual(expect.arrayContaining([...CANONICAL_RESET_KEYS]));
			expect(receivedCommands[1]?.length).toBe(6); // DEL + 5 keys

			// 3. QUIT
			expect(receivedCommands[2]).toEqual(["QUIT"]);
		} finally {
			await new Promise<void>((resolve) => server.close(() => resolve()));
		}
	});

	test("refuses invalid scheme or external hosts", () => {
		// Invalid scheme
		expect(
			canResetRedis({
				allowReset: true,
				redisUrl: "http://redis-streams:6379",
			}).allowed,
		).toBe(false);

		// External host in normal mode
		expect(
			canResetRedis({
				allowReset: true,
				redisUrl: "redis://evil-external-host.com:6379",
			}).allowed,
		).toBe(false);

		// External host even with allowLocalhostForTesting
		expect(
			canResetRedis({
				allowReset: true,
				allowLocalhostForTesting: true,
				redisUrl: "redis://evil-external-host.com:6379",
			}).allowed,
		).toBe(false);

		// Wrong port for redis-streams in normal mode
		expect(
			canResetRedis({
				allowReset: true,
				redisUrl: "redis://redis-streams:6380",
			}).allowed,
		).toBe(false);

		// Normalized IPv6 loopback allowed under allowLocalhostForTesting
		expect(
			canResetRedis({
				allowReset: true,
				allowLocalhostForTesting: true,
				redisUrl: "redis://[::1]:6379",
			}).allowed,
		).toBe(true);
	});

	test("rejects promptly on premature server EOF before QUIT", async () => {
		const server = net.createServer((socket) => {
			socket.on("data", () => {
				// Immediately close socket on receiving first byte without sending full response
				socket.destroy();
			});
		});

		await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
		const port = (server.address() as net.AddressInfo).port;

		try {
			await expect(
				resetCanonicalStreams({
					allowReset: true,
					allowLocalhostForTesting: true,
					redisUrl: `redis://127.0.0.1:${port}`,
				}),
			).rejects.toThrow(/Redis connection closed prematurely|Redis reset network error/);
		} finally {
			await new Promise<void>((resolve) => server.close(() => resolve()));
		}
	});

	test("rejects promptly on malformed RESP replies", async () => {
		const server = net.createServer((socket) => {
			socket.on("data", () => {
				// Reply with malformed non-integer response for DEL
				socket.write("+UNEXPECTED\r\n");
			});
		});

		await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
		const port = (server.address() as net.AddressInfo).port;

		try {
			await expect(
				resetCanonicalStreams({
					allowReset: true,
					allowLocalhostForTesting: true,
					redisUrl: `redis://127.0.0.1:${port}`,
				}),
			).rejects.toThrow(/Redis protocol error/);
		} finally {
			await new Promise<void>((resolve) => server.close(() => resolve()));
		}
	});

	test("assertCanonicalStreamsReset fails fast with clear message when reset is skipped/refused", async () => {
		// When opt-in is missing, assertCanonicalStreamsReset must throw to protect test isolation
		await expect(
			assertCanonicalStreamsReset({
				allowReset: false,
			}),
		).rejects.toThrow(/Redis stream reset was required for test isolation but did not execute/);
	});

	test("passwordFile validation: rejects missing/unreadable or empty files, accepts valid file", async () => {
		const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "redis-reset-test-"));
		const missingFile = path.join(tempDir, "does-not-exist.txt");
		const emptyFile = path.join(tempDir, "empty.txt");
		const validFile = path.join(tempDir, "valid.txt");

		fs.writeFileSync(emptyFile, "   \n");
		fs.writeFileSync(validFile, "fixture-secret-token\n");

		// 1. Missing password file throws sanitized configuration error without leaking path
		await expect(
			resetCanonicalStreams({
				allowReset: true,
				allowLocalhostForTesting: true,
				redisUrl: "redis://127.0.0.1:6379",
				passwordFile: missingFile,
			}),
		).rejects.toThrow("Configured REDIS_PASSWORD_FILE could not be read");

		// 2. Empty password file throws sanitized configuration error
		await expect(
			resetCanonicalStreams({
				allowReset: true,
				allowLocalhostForTesting: true,
				redisUrl: "redis://127.0.0.1:6379",
				passwordFile: emptyFile,
			}),
		).rejects.toThrow("Configured REDIS_PASSWORD_FILE is empty");

		// 3. Valid password file correctly authorizes with mock server
		let authArg = "";
		const server = net.createServer((socket) => {
			let buffer = Buffer.alloc(0);
			socket.on("data", (chunk) => {
				buffer = Buffer.concat([buffer, chunk]);
				while (buffer.length > 0) {
					if (buffer[0] !== 42) break;
					const crlf = buffer.indexOf("\r\n");
					if (crlf === -1) break;
					const numArgs = Number.parseInt(buffer.subarray(1, crlf).toString("utf8"), 10);
					let offset = crlf + 2;
					const args: string[] = [];
					let ok = true;
					for (let i = 0; i < numArgs; i++) {
						if (offset >= buffer.length || buffer[offset] !== 36) { ok = false; break; }
						const nextCrlf = buffer.indexOf("\r\n", offset);
						if (nextCrlf === -1) { ok = false; break; }
						const len = Number.parseInt(buffer.subarray(offset + 1, nextCrlf).toString("utf8"), 10);
						const s = nextCrlf + 2;
						const e = s + len;
						if (e + 2 > buffer.length) { ok = false; break; }
						args.push(buffer.subarray(s, e).toString("utf8"));
						offset = e + 2;
					}
					if (!ok) break;
					buffer = buffer.subarray(offset);
					const cmd = args[0]?.toUpperCase();
					if (cmd === "AUTH") {
						authArg = args[1] ?? "";
						socket.write("+OK\r\n");
					} else if (cmd === "DEL") {
						socket.write(`:${args.length - 1}\r\n`);
					} else if (cmd === "QUIT") {
						socket.write("+OK\r\n");
					}
				}
			});
		});

		await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
		const port = (server.address() as net.AddressInfo).port;

		try {
			const res = await resetCanonicalStreams({
				allowReset: true,
				allowLocalhostForTesting: true,
				redisUrl: `redis://127.0.0.1:${port}`,
				passwordFile: validFile,
			});
			expect(res.executed).toBe(true);
			expect(authArg).toBe("fixture-secret-token");
		} finally {
			await new Promise<void>((resolve) => server.close(() => resolve()));
			fs.rmSync(tempDir, { recursive: true, force: true });
		}
	});
});

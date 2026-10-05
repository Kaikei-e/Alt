/**
 * Ephemeral Redis stream isolation for mq-hub E2E tests.
 *
 * Implements a minimal, zero-dependency Node RESP client over net.Socket
 * specifically designed to reset canonical Redis streams in ephemeral CI runs.
 *
 * STRICT SAFETY GUARDS:
 * 1. Explicit opt-in required (MQ_E2E_ALLOW_REDIS_RESET=1).
 * 2. Protected localhost refusal: refuses to reset localhost / 127.0.0.1 / ::1
 *    (the user's running stack) unless explicitly running offline unit tests with mock.
 * 3. Key restriction: ONLY allowed canonical streams can ever be deleted.
 * 4. NEVER executes FLUSHALL or FLUSHDB.
 * 5. Secret redaction: never logs or echoes credentials.
 * 6. ACL parity: authenticates as the user REDIS_URL names and sends only
 *    commands on that user's allow-list in docker/redis/entrypoint.sh
 *    (AUTH, DEL). The `default` user is off there, so single-argument AUTH
 *    is never sent.
 */
import * as fs from "node:fs";
import * as net from "node:net";
import { URL } from "node:url";
import { CanonicalStream, env } from "./env.js";

/**
 * Allowed canonical stream keys that may be reset.
 * Deleting a Redis stream key also removes all consumer groups on that stream.
 */
export const CANONICAL_RESET_KEYS = [
	CanonicalStream.articles,
	CanonicalStream.summaries,
	CanonicalStream.tags,
	CanonicalStream.index,
	CanonicalStream.articlesDLQ,
] as const;

export const ALLOWED_CANONICAL_STREAMS = new Set<string>(CANONICAL_RESET_KEYS);

const PROTECTED_HOSTS = new Set(["localhost", "127.0.0.1", "::1", "0.0.0.0"]);

export function isProtectedHost(host: string): boolean {
	return PROTECTED_HOSTS.has(host.toLowerCase());
}

export interface RedisResetOptions {
	redisUrl?: string | undefined;
	password?: string | undefined;
	passwordFile?: string | undefined;
	allowReset?: boolean | undefined;
	allowLocalhostForTesting?: boolean | undefined;
	streamsToReset?: readonly string[] | undefined;
	timeoutMs?: number | undefined;
}

export interface ResetCheckResult {
	allowed: boolean;
	reason?:
		| "opt_in_required"
		| "redis_url_missing"
		| "invalid_redis_url"
		| "invalid_redis_scheme"
		| "target_not_allowed"
		| "localhost_refused"
		| undefined;
}

export interface ResetExecutionResult {
	executed: boolean;
	deletedCount?: number | undefined;
	reason?: string | undefined;
}

/**
 * Normalizes host strings, stripping IPv6 bracket formatting.
 */
function normalizeHostname(raw: string): string {
	return raw.replace(/^\[|\]$/g, "").toLowerCase();
}

function isLoopback(host: string): boolean {
	const normalized = normalizeHostname(host);
	return normalized === "localhost" || normalized === "127.0.0.1" || normalized === "::1" || normalized === "0.0.0.0";
}

/**
 * Guard that verifies whether Redis reset is permitted.
 * Strictly requires redis: scheme and dedicated slice target (redis-streams:6379),
 * or offline loopback mock under explicit allowLocalhostForTesting.
 */
export function canResetRedis(options: RedisResetOptions): ResetCheckResult {
	if (!options.allowReset) {
		return { allowed: false, reason: "opt_in_required" };
	}

	if (!options.redisUrl || options.redisUrl.trim() === "") {
		return { allowed: false, reason: "redis_url_missing" };
	}

	let parsedUrl: URL;
	try {
		parsedUrl = new URL(options.redisUrl);
	} catch {
		return { allowed: false, reason: "invalid_redis_url" };
	}

	if (parsedUrl.protocol !== "redis:") {
		return { allowed: false, reason: "invalid_redis_scheme" };
	}

	const normalizedHost = normalizeHostname(parsedUrl.hostname);
	const port = parsedUrl.port ? Number.parseInt(parsedUrl.port, 10) : 6379;

	if (isLoopback(normalizedHost)) {
		if (!options.allowLocalhostForTesting) {
			return { allowed: false, reason: "localhost_refused" };
		}
		// Under explicit allowLocalhostForTesting, loopback on ephemeral test ports is allowed
	} else if (normalizedHost !== "redis-streams" || port !== 6379) {
		// External hosts or unexpected ports are strictly rejected
		return { allowed: false, reason: "target_not_allowed" };
	}

	const targets = options.streamsToReset ?? CANONICAL_RESET_KEYS;
	for (const key of targets) {
		if (!ALLOWED_CANONICAL_STREAMS.has(key)) {
			throw new Error(`Refusing to reset non-canonical stream key: ${key}`);
		}
	}

	return { allowed: true };
}

/**
 * Encodes an array of string arguments into Redis Serialization Protocol (RESP).
 */
export function encodeRespCommand(args: readonly string[]): Buffer {
	const parts: (string | Buffer)[] = [`*${args.length}\r\n`];
	for (const arg of args) {
		const buf = Buffer.from(arg, "utf8");
		parts.push(`$${buf.length}\r\n`, buf, "\r\n");
	}
	return Buffer.concat(parts.map((p) => (typeof p === "string" ? Buffer.from(p, "utf8") : p)));
}

interface RedisCredentials {
	username: string;
	password: string;
}

/**
 * Executes AUTH <user> <password> (when credentials are given), then DEL for
 * the canonical stream keys, and closes the socket itself: QUIT is not on the
 * `streams` user's allow-list.
 */
function executeRespReset(
	host: string,
	port: number,
	targets: readonly string[],
	credentials: RedisCredentials | undefined,
	timeoutMs: number,
): Promise<number> {
	return new Promise((resolve, reject) => {
		const socket = new net.Socket();
		let buffer = Buffer.alloc(0);
		let step: "auth" | "del" = credentials ? "auth" : "del";
		let settled = false;

		const timer = setTimeout(() => {
			if (!settled) {
				socket.destroy(new Error("Redis reset connection timed out"));
			}
		}, timeoutMs);

		function cleanup() {
			clearTimeout(timer);
			socket.removeAllListeners();
			// The promise is settled; a reset arriving while the socket closes
			// must not become an unhandled 'error' event that kills the worker.
			socket.on("error", () => {});
		}

		socket.on("error", () => {
			if (!settled) {
				settled = true;
				cleanup();
				reject(new Error("Redis reset network error"));
			}
		});

		socket.on("close", () => {
			if (!settled) {
				settled = true;
				cleanup();
				reject(new Error("Redis connection closed prematurely"));
			}
		});

		socket.on("connect", () => {
			if (credentials) {
				socket.write(encodeRespCommand(["AUTH", credentials.username, credentials.password]));
			} else {
				socket.write(encodeRespCommand(["DEL", ...targets]));
			}
		});

		socket.on("data", (chunk) => {
			buffer = Buffer.concat([buffer, chunk]);
			while (true) {
				const crlf = buffer.indexOf("\r\n");
				if (crlf === -1) break;
				const line = buffer.subarray(0, crlf).toString("utf8");
				buffer = buffer.subarray(crlf + 2);

				if (line.startsWith("-")) {
					if (!settled) {
						settled = true;
						cleanup();
						socket.destroy();
						reject(new Error("Redis protocol error"));
					}
					return;
				}

				if (step === "auth") {
					if (line !== "+OK") {
						if (!settled) {
							settled = true;
							cleanup();
							socket.destroy();
							reject(new Error("Redis protocol error"));
						}
						return;
					}
					step = "del";
					socket.write(encodeRespCommand(["DEL", ...targets]));
				} else if (step === "del") {
					if (!line.startsWith(":") || !/^:\d+$/.test(line)) {
						if (!settled) {
							settled = true;
							cleanup();
							socket.destroy();
							reject(new Error("Redis protocol error"));
						}
						return;
					}
					const deletedCount = Number.parseInt(line.substring(1), 10);
					if (!settled) {
						settled = true;
						cleanup();
						socket.end();
						resolve(deletedCount);
					}
					return;
				}
			}
		});

		socket.connect(port, host);
	});
}

/**
 * Safely resets canonical Redis streams if explicitly allowed by configuration.
 * When running against default localhost or without opt-in, safely skips and returns { executed: false }.
 */
export async function resetCanonicalStreams(
	options?: Partial<RedisResetOptions>,
): Promise<ResetExecutionResult> {
	const resolved: RedisResetOptions = {
		allowReset: options?.allowReset ?? env.allowRedisReset,
		redisUrl: options?.redisUrl ?? env.redisUrl,
		password: options?.password,
		passwordFile: options?.passwordFile ?? env.redisPasswordFile,
		allowLocalhostForTesting: options?.allowLocalhostForTesting ?? false,
		streamsToReset: options?.streamsToReset ?? CANONICAL_RESET_KEYS,
		timeoutMs: options?.timeoutMs ?? 3_000,
	};

	const check = canResetRedis(resolved);
	if (!check.allowed) {
		return { executed: false, reason: check.reason };
	}

	const url = new URL(resolved.redisUrl!);
	const host = url.hostname;
	const port = Number.parseInt(url.port || "6379", 10);

	let password = resolved.password;
	if (!password && url.password) {
		password = decodeURIComponent(url.password);
	}
	if (!password && resolved.passwordFile && resolved.passwordFile.trim() !== "") {
		let raw: string;
		try {
			if (!fs.existsSync(resolved.passwordFile)) {
				throw new Error("not_found");
			}
			raw = fs.readFileSync(resolved.passwordFile, "utf8");
		} catch {
			throw new Error("Configured REDIS_PASSWORD_FILE could not be read");
		}
		password = raw.trim();
		if (password === "") {
			throw new Error("Configured REDIS_PASSWORD_FILE is empty");
		}
	}

	let credentials: RedisCredentials | undefined;
	if (password) {
		const username = decodeURIComponent(url.username);
		if (username === "") {
			throw new Error(
				"REDIS_URL must name the Redis ACL user (redis://<user>@host:port) when a password is configured",
			);
		}
		credentials = { username, password };
	}

	const targets = resolved.streamsToReset ?? CANONICAL_RESET_KEYS;
	const deletedCount = await executeRespReset(host, port, targets, credentials, resolved.timeoutMs!);
	return { executed: true, deletedCount };
}

/**
 * Asserts that canonical Redis streams were reset successfully.
 * Fails fast with a sanitized configuration error if reset was skipped or refused.
 */
export async function assertCanonicalStreamsReset(
	options?: Partial<RedisResetOptions>,
): Promise<ResetExecutionResult> {
	const result = await resetCanonicalStreams(options);
	if (!result.executed) {
		throw new Error(
			`Redis stream reset was required for test isolation but did not execute (reason: ${result.reason ?? "unknown"}). ` +
				`Ensure MQ_E2E_ALLOW_REDIS_RESET=1 and REDIS_URL are properly configured.`,
		);
	}
	return result;
}

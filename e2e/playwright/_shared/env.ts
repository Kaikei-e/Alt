/**
 * Suite configuration primitives.
 *
 * Every suite reads its endpoints from the environment and has **no
 * defaults**, deliberately. CLAUDE.md rule 9 forbids warn-and-limp startup
 * config, and a default URL is exactly that failure mode wearing a
 * test-harness hat: a suite pointed at a host that does not exist reports
 * "connection refused" on scenario 1 instead of "you forgot to export
 * BASE_URL", and a suite pointed at the *wrong* host reports green.
 * `run.sh` is the single place these are set.
 *
 * The one exception is `optionalEnv`, which exists for values that are
 * genuinely optional in the protocol sense (a stub hostname that only some
 * scenarios use). It takes an explicit fallback at the call site so the
 * default is visible in the suite's own env.ts rather than hidden here.
 */
import { readFileSync } from "node:fs";

/** Suffix appended to every "you forgot to set this" message. */
function hint(name: string): string {
	return (
		`${name} is not set. Alt's Playwright suites read every endpoint from ` +
		`the environment and have no defaults; run the suite through its ` +
		`e2e/playwright/<service>/run.sh, which exports them.`
	);
}

/** A required string. Throws — loudly, once — when unset or blank. */
export function requiredEnv(name: string): string {
	const value = process.env[name];
	if (value === undefined || value.trim() === "") {
		throw new Error(hint(name));
	}
	return value.trim();
}

/** An optional string with an explicit, call-site-visible fallback. */
export function optionalEnv(name: string, fallback: string): string {
	const value = process.env[name];
	return value === undefined || value.trim() === "" ? fallback : value.trim();
}

/** A required positive integer (ports, timeouts, counts). */
export function requiredIntEnv(name: string): number {
	const raw = requiredEnv(name);
	const value = Number.parseInt(raw, 10);
	if (!Number.isFinite(value)) {
		throw new Error(`${name} must be an integer (got: ${raw})`);
	}
	return value;
}

/**
 * A secret read from a file path rather than from the value itself.
 *
 * Tokens arrive as paths so they never land in `docker inspect` output, a
 * compose slice, or a CI job log's environment dump. Header values must not
 * contain CR/LF, hence the trim.
 */
export function requiredSecretFile(name: string): string {
	const path = requiredEnv(name);
	let raw: string;
	try {
		raw = readFileSync(path, "utf8");
	} catch (error) {
		const detail = error instanceof Error ? error.message : String(error);
		throw new Error(`${name} points at ${path}, which could not be read: ${detail}`);
	}
	const value = raw.trim();
	if (value === "") {
		throw new Error(`${name} points at ${path}, which is empty.`);
	}
	return value;
}

/**
 * A run identifier unique to this dispatch.
 *
 * Suites embed it in the names of the rows they create, which is what lets
 * `fullyParallel` workers — and two shards on one daemon — share a database
 * without ever seeing each other's data. Defaulting to "local" is safe
 * because a local run owns the whole stack.
 */
export function runId(): string {
	return optionalEnv("RUN_ID", "local");
}

/** The all-zero UUID: a stable "definitely not a row of mine" probe. */
export const ZERO_UUID = "00000000-0000-0000-0000-000000000000";

/**
 * Suite configuration, read once per process (config file, global setup, and
 * every worker each evaluate this module).
 *
 * Everything here is *required*. There are no defaults, deliberately:
 * CLAUDE.md rule 9 forbids warn-and-limp startup config, and a default URL is
 * exactly that failure mode wearing a test-harness hat — a suite pointed at a
 * host that does not exist reports "connection refused" on scenario 1 instead
 * of "you forgot to export BASE_URL", and a suite pointed at the *wrong* host
 * reports green. `run.sh` is the single place these are set.
 */
import { readFileSync } from "node:fs";

function required(name: string): string {
	const value = process.env[name];
	if (value === undefined || value.trim() === "") {
		throw new Error(
			`${name} is not set. The alt-backend Playwright suite reads every ` +
				`endpoint from the environment and has no defaults; run it through ` +
				`e2e/playwright/alt-backend/run.sh, which exports them.`,
		);
	}
	return value.trim();
}

/**
 * The JWT arrives as a path rather than a value so it never lands in
 * `docker inspect` output or a compose slice. Header values must not contain
 * CR/LF, hence the trim.
 */
function requiredSecretFile(name: string): string {
	const path = required(name);
	const value = readFileSync(path, "utf8").trim();
	if (value === "") {
		throw new Error(`${name} points at ${path}, which is empty.`);
	}
	return value;
}

export const env = {
	/** User-facing REST listener (Echo). */
	baseURL: required("BASE_URL"),
	/** User-facing Connect-RPC listener. */
	connectURL: required("CONNECT_URL"),
	/** Loopback-bound operator listener that keeps the admin Connect services. */
	internalURL: required("INTERNAL_URL"),
	/** Shared operator listener: /health + /metrics for all three split binaries. */
	opsURL: required("OPS_URL"),
	/** Hostname alt-backend's SSRF allowlist (FEED_ALLOWED_HOSTS) admits. */
	stubHost: process.env.STUB_HOST?.trim() || "stub.invalid",
	/** Pre-minted HS256 token: role=admin, exp=2099-01-01. */
	jwt: requiredSecretFile("JWT_FILE"),
	/** Unique per dispatch; only used to keep report/artifact paths apart. */
	runId: process.env.RUN_ID?.trim() || "local",
} as const;

/** A JWT that parses but was signed with a different secret. */
export const WRONG_SIGNATURE_JWT =
	"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
	"eyJzdWIiOiIwMDAwMDAwMC0wMDAwLTAwMDAtMDAwMC0wMDAwMDAwMDAwMDEiLCJ0ZW5hbnRfaWQiOiIwMDAwMDAwMC0wMDAwLTAwMDAtMDAwMC0wMDAwMDAwMDAwMDEiLCJpc3MiOiJhbHQtc3RhZ2luZy1hdXRoLWh1YiIsImF1ZCI6ImFsdC1iYWNrZW5kIiwiZXhwIjo0MDcwOTA4ODAwfQ." +
	"WRONG_SIGNATURE_xxxx";

/** The all-zero UUID: a stable "definitely not a row of mine" probe. */
export const ZERO_UUID = "00000000-0000-0000-0000-000000000000";

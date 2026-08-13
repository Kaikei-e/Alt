/**
 * Server-side client for knowledge-sovereign admin REST endpoints.
 *
 * Calls knowledge-sovereign metrics port (:9501) directly from SvelteKit server,
 * authenticates the call, and hands the bodies to $lib/server/sovereign-admin-wire
 * for the envelope unwrapping and the snake_case → camelCase rename.
 */

import { readFileSync } from "node:fs";
import { env } from "$env/dynamic/private";
import type {
	RawRetentionRun,
	SovereignAdminWire,
} from "$lib/server/sovereign-admin-wire";
import {
	normalizeRetentionRun,
	normalizeSnapshotMetadata,
	normalizeSovereignAdminSnapshot,
} from "$lib/server/sovereign-admin-wire";
import type {
	RetentionRunResponse,
	SnapshotMetadata,
	SovereignAdminSnapshot,
} from "$lib/types/sovereign-admin";

const SOVEREIGN_METRICS_URL =
	env.SOVEREIGN_METRICS_URL || "http://knowledge-sovereign:9501";

// knowledge-sovereign Bearer-gates every /admin/* route on its metrics port and
// opens it only for an explicit ADMIN_AUTH=disabled, so this caller mirrors that
// switch rather than reading an absent token as "no token needed".
function loadAdminToken(): string | null {
	if (env.SOVEREIGN_ADMIN_AUTH === "disabled") {
		console.warn(
			"sovereign_admin_auth_disabled: SOVEREIGN_ADMIN_AUTH=disabled was set explicitly; /admin/* calls carry no Bearer token",
		);
		return null;
	}

	const tokenFile = env.SOVEREIGN_ADMIN_TOKEN_FILE;
	if (tokenFile) {
		let contents: string;
		try {
			contents = readFileSync(tokenFile, "utf8");
		} catch (cause) {
			// This runs at server startup, where a bare fs error reaches the
			// operator only as a container that exited: name the config key, the
			// path and the OS reason so the log line alone is diagnosable.
			throw new Error(
				`SOVEREIGN_ADMIN_TOKEN_FILE (${tokenFile}) could not be read: ${
					cause instanceof Error ? cause.message : String(cause)
				}`,
				{ cause },
			);
		}
		const token = contents.trim();
		if (!token) {
			throw new Error(`SOVEREIGN_ADMIN_TOKEN_FILE (${tokenFile}) is empty`);
		}
		console.info("sovereign_admin_auth_enabled");
		return token;
	}

	const token = env.SOVEREIGN_ADMIN_TOKEN?.trim();
	if (token) {
		console.info("sovereign_admin_auth_enabled");
		return token;
	}

	throw new Error(
		"SOVEREIGN_ADMIN_TOKEN_FILE or SOVEREIGN_ADMIN_TOKEN is required; set SOVEREIGN_ADMIN_AUTH=disabled to call knowledge-sovereign /admin/* without a token",
	);
}

let resolved: { token: string | null } | undefined;

// Resolved on first use, never at module load: `vite build` imports every server
// module to analyse the routes, and the machine building the image holds no
// runtime secrets. The server hook below moves that first use to process start.
function adminToken(): string | null {
	resolved ??= { token: loadAdminToken() };
	return resolved.token;
}

// Called from the `init` server hook so that a missing token kills the process
// at startup instead of surfacing as 401s the first time someone opens the panel.
export function verifySovereignAdminAuth(): void {
	adminToken();
}

function adminHeaders(
	extra?: Record<string, string>,
): Record<string, string> | undefined {
	const token = adminToken();
	if (!token) {
		return extra;
	}
	return { ...extra, Authorization: `Bearer ${token}` };
}

async function fetchJSON<T>(url: string): Promise<T> {
	const response = await fetch(url, { headers: adminHeaders() });
	if (!response.ok) {
		throw new Error(`Sovereign API error: ${response.status} ${url}`);
	}
	return response.json() as Promise<T>;
}

export async function fetchSovereignAdminSnapshot(): Promise<SovereignAdminSnapshot> {
	const [storage, snapshotList, latestSnapshot, retentionStatus, eligible] =
		await Promise.all([
			fetchJSON<SovereignAdminWire["storage"]>(
				`${SOVEREIGN_METRICS_URL}/admin/storage/stats`,
			),
			fetchJSON<SovereignAdminWire["snapshotList"]>(
				`${SOVEREIGN_METRICS_URL}/admin/snapshots/list`,
			),
			fetchJSON<SovereignAdminWire["latestSnapshot"]>(
				`${SOVEREIGN_METRICS_URL}/admin/snapshots/latest`,
			),
			fetchJSON<SovereignAdminWire["retentionStatus"]>(
				`${SOVEREIGN_METRICS_URL}/admin/retention/status`,
			),
			fetchJSON<SovereignAdminWire["eligible"]>(
				`${SOVEREIGN_METRICS_URL}/admin/retention/eligible`,
			),
		]);

	return normalizeSovereignAdminSnapshot({
		storage,
		snapshotList,
		latestSnapshot,
		retentionStatus,
		eligible,
	});
}

export async function createSovereignSnapshot(): Promise<SnapshotMetadata> {
	const response = await fetch(
		`${SOVEREIGN_METRICS_URL}/admin/snapshots/create`,
		{ method: "POST", headers: adminHeaders() },
	);
	if (!response.ok) {
		throw new Error(`Failed to create snapshot: ${response.status}`);
	}
	return normalizeSnapshotMetadata(await response.json());
}

export async function runSovereignRetention(
	dryRun: boolean,
): Promise<RetentionRunResponse> {
	const response = await fetch(`${SOVEREIGN_METRICS_URL}/admin/retention/run`, {
		method: "POST",
		headers: adminHeaders({ "Content-Type": "application/json" }),
		body: JSON.stringify({ dry_run: dryRun }),
	});
	if (!response.ok) {
		throw new Error(`Failed to run retention: ${response.status}`);
	}
	return normalizeRetentionRun((await response.json()) as RawRetentionRun);
}

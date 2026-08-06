import { request } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";

/**
 * Readiness gating for `globalSetup` — the direct replacement for the
 * `00-setup.hurl` scenario each suite used to open with.
 *
 * `run.sh` already waits on compose healthchecks, but a healthy container is
 * not a ready service: a Go binary reports healthy once its listeners bind,
 * while an mTLS handshake, a first migration, a Meilisearch index creation or
 * a stub's first import can still be settling. Probing here rather than inside
 * a spec means a stack that never comes up fails **once**, with one legible
 * message naming the probe that never passed — instead of failing every test
 * in the suite with a connection error and leaving the reader to work out
 * which of them was the cause.
 *
 * This is also why `00-setup.hurl` should not be ported as a test: a readiness
 * check that lives in the suite is order-dependent by construction, and
 * `fullyParallel` has no notion of "run this one first".
 */

/** Generous: a cold migrator plus a model warmup can take most of a minute. */
const DEFAULT_TIMEOUT_MS = 90_000;
const DEFAULT_INTERVAL_MS = 1_000;

export type Probe = {
	/** What this proves, in prose. Shown verbatim when it never passes. */
	readonly label: string;
	/** Throws (or rejects) while not ready; returns when ready. */
	readonly run: (api: APIRequestContext) => Promise<void>;
};

export type WaitOptions = {
	readonly timeout?: number;
	readonly interval?: number;
};

async function waitFor(
	api: APIRequestContext,
	probe: Probe,
	options: WaitOptions,
): Promise<void> {
	const timeout = options.timeout ?? DEFAULT_TIMEOUT_MS;
	const interval = options.interval ?? DEFAULT_INTERVAL_MS;
	const deadline = Date.now() + timeout;
	let lastError: unknown;

	while (Date.now() < deadline) {
		try {
			await probe.run(api);
			return;
		} catch (error) {
			lastError = error;
			await new Promise((resolve) => setTimeout(resolve, interval));
		}
	}

	const detail = lastError instanceof Error ? lastError.message : String(lastError);
	throw new Error(
		`readiness probe "${probe.label}" did not pass within ${timeout}ms. ` +
			`Last failure: ${detail}`,
	);
}

/**
 * Runs every probe in order, then disposes the context.
 *
 * Serial on purpose: the probes are usually a dependency chain (database →
 * service → the service's own view of the database), and a parallel run would
 * report the last link's failure while the first is still the cause.
 */
export async function waitForReady(
	probes: readonly Probe[],
	options: WaitOptions = {},
): Promise<void> {
	const api = await request.newContext();
	try {
		for (const probe of probes) {
			await waitFor(api, probe, options);
		}
	} finally {
		await api.dispose();
	}
}

/**
 * The commonest probe: a GET that must answer 2xx.
 *
 * Note it asserts `ok()`, not "any response". A service answering 503 because
 * its database is unreachable is precisely the state this gate exists to wait
 * through, not to accept.
 */
export function httpOk(url: string, label = `GET ${url}`): Probe {
	return {
		label,
		run: async (api) => {
			const response = await api.get(url, { timeout: 10_000 });
			if (!response.ok()) {
				throw new Error(`status ${response.status()}: ${(await response.text()).slice(0, 300)}`);
			}
		},
	};
}

/**
 * A GET whose body must also satisfy `check`.
 *
 * Use where "answers 200" is not the same as "is ready" — a health handler
 * that reports `{"status":"degraded","database":"disconnected"}` under a 200
 * is the usual case, and a suite that starts against it fails everywhere at
 * once for a reason that has nothing to do with the tests.
 */
export function httpBody(
	url: string,
	check: (body: unknown) => boolean,
	label = `GET ${url} (body)`,
): Probe {
	return {
		label,
		run: async (api) => {
			const response = await api.get(url, { timeout: 10_000 });
			if (!response.ok()) {
				throw new Error(`status ${response.status()}`);
			}
			const body: unknown = await response.json();
			if (!check(body)) {
				throw new Error(`unexpected body ${JSON.stringify(body).slice(0, 300)}`);
			}
		},
	};
}

/**
 * A Connect-RPC listener probe.
 *
 * Deliberately accepts **any** HTTP status in 4xx/5xx as well as 2xx: the fact
 * being established is that the listener answers at all. A connection error
 * throws and keeps polling; an `unauthenticated` or an `invalid_argument` both
 * prove the mux is up, which is all a readiness gate needs to know.
 */
export function connectListening(baseURL: string, procedure: string): Probe {
	const url = `${baseURL}${procedure.startsWith("/") ? procedure : `/${procedure}`}`;
	return {
		label: `Connect ${url}`,
		run: async (api) => {
			const response = await api.post(url, {
				headers: { "Content-Type": "application/json" },
				data: {},
				timeout: 10_000,
			});
			if (response.status() === 0) {
				throw new Error("no response");
			}
		},
	};
}

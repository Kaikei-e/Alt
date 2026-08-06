import { test as base } from "@playwright/test";
import type { APIRequestContext, PlaywrightWorkerArgs } from "@playwright/test";
import { env } from "./env.js";
import {
	kratosIdentitySchema,
	kratosLoginFlowSchema,
	kratosLoginResultSchema,
} from "./schemas.js";

/**
 * Suite-wide fixtures.
 *
 * The single biggest change from the Hurl suite is here. `00-setup.hurl`
 * created *one* Kratos identity with a fixed email, logged it in once, and
 * captured `kratos_session_cookie` / `user_id` / `session_id`; `run.sh` then
 * parsed those out of a JSON report and injected them into every later
 * scenario as `--variable`. That is why `--jobs 1` was load-bearing and why
 * the whole suite was one ordered chain: scenario 12 could not run without
 * scenario 0.
 *
 * Here every worker mints its own identity and its own session, and any test
 * that needs a *second* session calls `mintSession` and gets one. Nothing is
 * shared, nothing is ordered, and `fullyParallel` is safe. Isolation comes
 * from naming — `local+<runId>-w<n>-<rand>@domain` — not from teardown; the
 * staging slice is destroyed with `docker compose down -v` per dispatch, and a
 * teardown would only buy a race with a sibling worker's `whoami`.
 *
 * The second change is the client address. All four of auth-hub's rate
 * limiters (`middleware/rate_limit.go`) key on `c.RealIP()`, and main.go sets
 * `e.IPExtractor = echo.ExtractIPFromXFFHeader()`. Four workers sharing the
 * test container's single source address would share one token bucket, and the
 * /session bucket is burst 5 refilling at 0.5/s — the suite would 429 into
 * itself within seconds. Every test therefore gets its own synthetic
 * `X-Forwarded-For`, which is also what production looks like behind nginx.
 * tests/rate-limit.spec.ts asserts that this isolation is real rather than
 * assumed.
 */

export type SeededSession = {
	/** The Kratos identity's email — matches `X-Alt-User-Email` from /validate. */
	readonly email: string;
	/** Kratos identity id. auth-hub reports it as both user and tenant. */
	readonly userId: string;
	/** Kratos's own session UUID, from the login response. */
	readonly kratosSessionId: string;
	/** The `ory_kratos_session` cookie value — the credential under test. */
	readonly cookie: string;
	/** Ready-made `Cookie:` header value. */
	readonly cookieHeader: string;
};

/**
 * `local+<token>@domain`, derived from the fixture address in
 * `e2e/fixtures/auth-hub/test-identity-email.txt`.
 *
 * Sub-addressing keeps every seeded identity inside the fixture's own
 * `.invalid` domain, so nothing this suite creates can ever be a deliverable
 * address, while still giving Kratos the distinct `password` identifier it
 * needs to treat them as separate identities.
 */
function emailFor(token: string): string {
	const at = env.identityEmail.lastIndexOf("@");
	if (at <= 0) {
		throw new Error(
			`IDENTITY_EMAIL_FILE does not contain an email address: ${env.identityEmail}`,
		);
	}
	return `${env.identityEmail.slice(0, at)}+${token}@${env.identityEmail.slice(at + 1)}`;
}

/** Lowercase, URL- and email-safe, unique to (dispatch, worker, call). */
function seedToken(workerIndex: number): string {
	const random = Math.floor(Math.random() * 0xffffffff)
		.toString(16)
		.padStart(8, "0");
	return `${env.runId}-w${workerIndex}-${random}`.toLowerCase().replace(/[^a-z0-9-]/g, "");
}

/**
 * An address in RFC 6598 shared address space (100.64.0.0/10).
 *
 * Chosen because Echo's `ExtractIPFromXFFHeader` walks the forwarded chain
 * right-to-left and returns the first address it does *not* trust, where
 * "trusted" is loopback, link-local and RFC 1918 private space. 100.64/10 is
 * none of those, so it survives the walk — while being guaranteed never to be
 * a real client. Using a 10.x address here would be silently discarded and
 * every test would fall back to sharing the container's own bucket.
 */
export function syntheticClientIP(workerIndex: number): string {
	const second = 64 + (workerIndex % 64);
	const third = Math.floor(Math.random() * 256);
	const fourth = 1 + Math.floor(Math.random() * 254);
	return `100.${second}.${third}.${fourth}`;
}

/**
 * Creates a Kratos identity and logs it in, returning the session cookie.
 *
 * The flow is the one `00-setup.hurl` documented at length, and the reasoning
 * survives intact: the **browser** flow is the only path in Kratos v1.3 that
 * issues the HMAC-signed `ory_kratos_session` cookie auth-hub's handlers
 * parse. The `api` flow's `session_token` is opaque and is not a valid cookie
 * value for a self-hosted Kratos, despite `whoami` accepting it as a bearer
 * token — a distinction the Hurl README got wrong in prose and right in code.
 *
 * `Accept: application/json` on the init makes Kratos answer with the flow
 * JSON instead of a 303 to the UI stub, while still setting the per-flow
 * `csrf_token_<hash>` cookie. Playwright's `APIRequestContext` keeps its own
 * cookie jar, so that cookie rides the submit automatically — the same
 * property Hurl's jar provided.
 */
async function seedSession(
	playwright: PlaywrightWorkerArgs["playwright"],
	token: string,
): Promise<SeededSession> {
	const email = emailFor(token);

	const admin = await playwright.request.newContext({ baseURL: env.kratosAdminURL });
	try {
		const created = await admin.post("/admin/identities", {
			headers: { "Content-Type": "application/json" },
			data: {
				schema_id: "default",
				traits: { email },
				credentials: { password: { config: { password: env.identityPassword } } },
				state: "active",
			},
		});
		// 00-setup.hurl accepted 200/201/409 because it always used the same
		// fixed email and had to be idempotent across a re-run against a warm
		// volume. This suite mints a unique address per call, so a 409 would mean
		// two seeds collided — a real defect in `seedToken`, not a re-run. 200 and
		// 201 are both accepted because Kratos v1.3 documents 201 while adjacent
		// minor releases have answered 200; both mean "created".
		if (created.status() !== 200 && created.status() !== 201) {
			throw new Error(
				`seeding identity ${email} failed: POST /admin/identities -> ` +
					`${created.status()} ${(await created.text()).slice(0, 500)}`,
			);
		}
		kratosIdentitySchema.parse(await created.json());
	} finally {
		await admin.dispose();
	}

	const publicApi = await playwright.request.newContext({ baseURL: env.kratosPublicURL });
	try {
		const init = await publicApi.get("/self-service/login/browser", {
			headers: { Accept: "application/json" },
		});
		if (init.status() !== 200) {
			throw new Error(
				`GET /self-service/login/browser -> ${init.status()} ` +
					`${(await init.text()).slice(0, 500)}`,
			);
		}
		const flow = kratosLoginFlowSchema.parse(await init.json());

		// Hurl reached the token as `$.ui.nodes[0].attributes.value` — positional,
		// so a Kratos release that reorders the UI nodes would have submitted some
		// other field's value as the CSRF token and failed with a confusing 400.
		const csrfNode = flow.ui.nodes.find((node) => node.attributes.name === "csrf_token");
		const csrfToken = csrfNode?.attributes.value;
		if (typeof csrfToken !== "string" || csrfToken === "") {
			throw new Error(
				`login flow ${flow.id} exposed no csrf_token node: ` +
					`${JSON.stringify(flow.ui.nodes).slice(0, 500)}`,
			);
		}

		const submitted = await publicApi.post(flow.ui.action, {
			headers: { Accept: "application/json", "Content-Type": "application/json" },
			data: {
				method: "password",
				identifier: email,
				password: env.identityPassword,
				csrf_token: csrfToken,
			},
		});
		if (submitted.status() !== 200) {
			throw new Error(
				`login for ${email} failed: POST ${flow.ui.action} -> ${submitted.status()} ` +
					`${(await submitted.text()).slice(0, 500)}`,
			);
		}
		const result = kratosLoginResultSchema.parse(await submitted.json());
		if (result.session.identity.traits.email !== email) {
			throw new Error(
				`login returned a session for ${result.session.identity.traits.email}, ` +
					`not for ${email}`,
			);
		}

		const state = await publicApi.storageState();
		const cookie = state.cookies.find((entry) => entry.name === "ory_kratos_session");
		if (cookie === undefined || cookie.value === "") {
			throw new Error(
				`login for ${email} set no ory_kratos_session cookie; cookies: ` +
					`${state.cookies.map((entry) => entry.name).join(", ") || "<none>"}`,
			);
		}

		return {
			email,
			userId: result.session.identity.id,
			kratosSessionId: result.session.id,
			cookie: cookie.value,
			cookieHeader: `ory_kratos_session=${cookie.value}`,
		};
	} finally {
		await publicApi.dispose();
	}
}

type WorkerFixtures = {
	/** Kratos AdminAPI :4434 — identity reads and session revocation. */
	kratosAdmin: APIRequestContext;
	/**
	 * One identity + live session per worker.
	 *
	 * Worker-scoped rather than test-scoped because a session is expensive
	 * (an admin write plus a bcrypt login) and nothing about it is mutated by
	 * the read-only paths that consume it. Tests that revoke or otherwise
	 * destroy a session must call `mintSession` for their own.
	 */
	session: SeededSession;
};

type TestFixtures = {
	/** This test's private client address — see the module comment. */
	clientIP: string;
	/** auth-hub :8888, anonymous, carrying this test's client address. */
	hub: APIRequestContext;
	/**
	 * Builds an additional auth-hub client bound to an explicit address.
	 *
	 * Only tests/rate-limit.spec.ts needs it: proving the limiter is keyed per
	 * IP requires two addresses inside one test. Contexts are disposed with the
	 * test.
	 */
	hubFrom: (ip: string) => Promise<APIRequestContext>;
	/**
	 * Mints a brand-new identity + session, for tests that consume one.
	 *
	 * A test that revokes a session, or that needs two sessions to prove a
	 * token is session-bound, cannot borrow the worker's — it would poison every
	 * later test on that worker. This is the "seed what you read" rule with
	 * teeth.
	 */
	mintSession: () => Promise<SeededSession>;
};

export const test = base.extend<TestFixtures, WorkerFixtures>({
	kratosAdmin: [
		async ({ playwright }, use) => {
			const context = await playwright.request.newContext({ baseURL: env.kratosAdminURL });
			await use(context);
			await context.dispose();
		},
		{ scope: "worker" },
	],

	session: [
		async ({ playwright }, use, workerInfo) => {
			await use(await seedSession(playwright, seedToken(workerInfo.workerIndex)));
		},
		{ scope: "worker" },
	],

	clientIP: async ({}, use, testInfo) => {
		// Test-scoped, not worker-scoped as in alt-backend's suite. auth-hub's
		// /internal limiter is burst 3 at 10 req/min and the limiter runs *before*
		// the auth middleware (main.go:183-187), so three probes — however they
		// are authenticated — exhaust the bucket for six seconds. A worker-scoped
		// address would make tests/internal.spec.ts order-dependent in exactly the
		// way `fullyParallel` forbids. A retry re-evaluates this fixture, so a
		// retried test also starts from a full bucket.
		await use(syntheticClientIP(testInfo.workerIndex));
	},

	hub: async ({ playwright, clientIP }, use) => {
		const context = await playwright.request.newContext({
			baseURL: env.baseURL,
			extraHTTPHeaders: { "X-Forwarded-For": clientIP },
		});
		await use(context);
		await context.dispose();
	},

	hubFrom: async ({ playwright }, use) => {
		const created: APIRequestContext[] = [];
		await use(async (ip: string) => {
			const context = await playwright.request.newContext({
				baseURL: env.baseURL,
				extraHTTPHeaders: { "X-Forwarded-For": ip },
			});
			created.push(context);
			return context;
		});
		for (const context of created) {
			await context.dispose();
		}
	},

	mintSession: async ({ playwright }, use, testInfo) => {
		await use(async () => seedSession(playwright, seedToken(testInfo.workerIndex)));
	},
});

export { expect } from "@playwright/test";

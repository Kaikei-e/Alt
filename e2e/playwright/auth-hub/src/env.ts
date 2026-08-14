/**
 * Suite configuration, read once per process (the config file, global setup
 * and every worker each evaluate this module).
 *
 * Everything here is *required*. There are no defaults, deliberately:
 * CLAUDE.md rule 9 forbids warn-and-limp startup config, and a default URL is
 * that failure mode wearing a test-harness hat — a suite pointed at a host
 * that does not exist reports "connection refused" on the first spec instead
 * of "you forgot to export BASE_URL", and a suite pointed at the *wrong* host
 * reports green. `run.sh` is the single place these are set.
 */
import { requiredEnv, requiredIntEnv, requiredSecretFile, runId } from "../../_shared/env.js";

export const env = {
	/** auth-hub's only listener in staging: Echo on :8888 (compose `PORT=8888`). */
	baseURL: requiredEnv("BASE_URL"),

	/**
	 * Kratos FrontendAPI :4433 — where the self-service login flow lives.
	 *
	 * The suite talks to Kratos directly to seed identities and mint sessions.
	 * That is not a shortcut around auth-hub: auth-hub has no login endpoint at
	 * all, so an `ory_kratos_session` cookie can only come from Kratos.
	 */
	kratosPublicURL: requiredEnv("KRATOS_PUBLIC_URL"),

	/** Kratos AdminAPI :4434 — identity create, identity read, session revoke. */
	kratosAdminURL: requiredEnv("KRATOS_ADMIN_URL"),

	/**
	 * The address auth-hub's optional mutual-TLS listener would bind to.
	 *
	 * `cmd/auth-hub/main.go` only starts it under `MTLS_LISTEN=true`, and
	 * compose.staging.yaml sets `MTLS_LISTEN=false`. tests/topology.spec.ts
	 * asserts nothing is listening here — the negative half of a feature flag
	 * that no unit test can observe.
	 */
	mtlsURL: requiredEnv("MTLS_URL"),

	/**
	 * The HS256 secret auth-hub signs backend JWTs with — and nothing else.
	 *
	 * It used to do two jobs: `main.go` keyed the /internal group on this same
	 * value. It no longer does (`wireInternalAuth(cfg)` reads
	 * `INTERNAL_AUTH_SECRET`), because the /internal bearer crosses the network
	 * in a plaintext header and lands in nginx access logs and OTel span
	 * attributes, which is no place for a signing key. auth-hub now refuses to
	 * start when the two are equal, so sending this one to /internal is a 403.
	 * See `internalAuthSecret` below.
	 *
	 * Arrives as a path rather than a value so it never lands in `docker
	 * inspect` output, a rendered compose slice, or a CI environment dump.
	 */
	backendTokenSecret: requiredSecretFile("BACKEND_TOKEN_SECRET_FILE"),

	/**
	 * The shared bearer auth-hub's /internal group compares `X-Internal-Auth`
	 * against (`middleware/internal_auth.go`, keyed on `INTERNAL_AUTH_SECRET`).
	 *
	 * A plain value rather than a path, unlike every other secret here, because
	 * compose.staging.yaml sets it inline — it is a throwaway staging literal
	 * that both auth-hub and alt-data-hub carry, and one literal is what keeps
	 * the three in step. Reading it from the environment rather than hard-coding
	 * it means a changed compose slice fails as a named 403 here instead of
	 * passing against a secret nothing uses.
	 */
	internalAuthSecret: requiredEnv("INTERNAL_AUTH_SECRET"),

	/**
	 * The Kratos identity fixtures the Hurl suite used verbatim.
	 *
	 * Read through `requiredSecretFile` for the file-read + trim + non-empty
	 * check, not because an email is secret — the password genuinely is, and
	 * treating both the same way keeps one code path.
	 *
	 * The email is a *template*: this suite seeds one identity per worker (and
	 * more inside individual tests) under `local+<token>@domain`, so the fleet
	 * can run `fullyParallel` against one Kratos. The Hurl suite could only
	 * ever have the single fixed identity, which is exactly why it needed
	 * `--jobs 1`.
	 */
	identityEmail: requiredSecretFile("IDENTITY_EMAIL_FILE"),
	identityPassword: requiredSecretFile("IDENTITY_PASSWORD_FILE"),

	/**
	 * The JWT claim values compose.staging.yaml configures auth-hub with
	 * (`BACKEND_TOKEN_ISSUER`, `BACKEND_TOKEN_AUDIENCE`, `BACKEND_TOKEN_TTL`).
	 *
	 * Read from the environment rather than hard-coded so that changing the
	 * compose slice and forgetting the suite produces a named mismatch here
	 * instead of a green run asserting last year's contract. alt-backend
	 * verifies exactly these three, so a drift is a cross-service outage.
	 */
	jwtIssuer: requiredEnv("JWT_ISSUER"),
	jwtAudience: requiredEnv("JWT_AUDIENCE"),
	jwtTTLSeconds: requiredIntEnv("JWT_TTL_SECONDS"),

	/** Unique per dispatch; embedded in seeded identity emails. */
	runId: runId(),
} as const;

/**
 * A well-formed but never-issued session cookie value.
 *
 * `04-validate-bad-cookie.hurl` used this exact string. It matters that it is
 * *parseable* as a cookie: it makes the request reach `usecase.
 * ValidateSession` and the Kratos `whoami` round-trip, which is a different
 * branch from the missing-cookie short-circuit in `validate.go`.
 */
export const INVALID_SESSION_VALUE = "deadbeef-not-a-valid-session-token-value";

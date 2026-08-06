/**
 * Suite configuration, read once per process (the config file, global setup
 * and every worker each evaluate this module).
 *
 * Everything here is *required*. There are no defaults, deliberately:
 * CLAUDE.md rule 9 forbids warn-and-limp startup config, and a default URL is
 * that same failure mode wearing a test-harness hat — a suite pointed at a
 * host that does not exist reports "connection refused" on scenario 1 instead
 * of "you forgot to export BASE_URL", and a suite pointed at the *wrong* host
 * reports green. `run.sh` is the single place these are set.
 */
import { requiredEnv, requiredSecretFile, runId } from "../../_shared/env.js";

export const env = {
	/** Plaintext REST listener: `/health` + `/v1/search` (`config.HTTPAddr`, default :9300). */
	baseURL: requiredEnv("BASE_URL"),

	/**
	 * Plaintext Connect-RPC listener (`config.ConnectAddr`, default :9301).
	 *
	 * h2c in production, but connect-go serves unary JSON over HTTP/1.1 too,
	 * which is what `APIRequestContext` speaks. That is the whole reason the
	 * Hurl suite listed this port as "out of scope" and this one does not.
	 */
	connectURL: requiredEnv("CONNECT_URL"),

	/**
	 * The port `bootstrap/app.go` binds the mutual-TLS mux on — but only when
	 * `MTLS_LISTEN == "true"`. compose.staging.yaml sets `MTLS_LISTEN=false`,
	 * so nothing may answer here; tests/topology.spec.ts asserts that rather
	 * than assuming it.
	 */
	mtlsAbsentURL: requiredEnv("MTLS_ABSENT_URL"),

	/** Meilisearch, which both the seed and the "is the key enforced" negative talk to. */
	meiliURL: requiredEnv("MEILI_URL"),

	/** Unique per dispatch; keeps each worker's seeded document ids apart across reruns. */
	runId: runId(),
} as const;

/**
 * The Meilisearch master key, and where the canonical fixture corpus lives.
 *
 * A function rather than a module-level const because `requiredSecretFile`
 * touches the filesystem: at module scope every worker would re-read the key
 * at import time, and a permissions problem on that file would fail the whole
 * suite with a confusing error rather than failing the seed that depends on
 * it.
 */
export function meiliEnv() {
	return {
		url: env.meiliURL,
		masterKey: requiredSecretFile("MEILI_MASTER_KEY_FILE"),
	} as const;
}

/** Only `setup/global-setup.ts` needs the shared corpus path. */
export function seedDocsPath(): string {
	return requiredEnv("MEILI_SEED_DOCS");
}

/**
 * Fully-qualified Connect procedures, as `services/search/v2/search.proto`
 * declares them.
 *
 * A const object rather than free strings so a typo is a compile error
 * instead of a test that reports green because it probed a path nothing was
 * ever mounted at.
 */
export const Procedure = {
	searchArticles: "services.search.v2.SearchService/SearchArticles",
	searchRecaps: "services.search.v2.SearchService/SearchRecaps",
	/** Registered service, unregistered method — the connect-go mux's own 404 arm. */
	unknownMethod: "services.search.v2.SearchService/NoSuchProcedure",
	/** Unregistered service — Go's `http.ServeMux` 404 arm, one layer further out. */
	unknownService: "services.search.v2.NoSuchService/SearchArticles",
} as const;

/**
 * The two documents in `e2e/fixtures/search-indexer/seed-docs.json` that carry
 * the word "rust", and the users that own them.
 *
 * Named here rather than re-derived in each spec because the parity
 * assertions inherited from the Hurl suite (`02`, `03`, `04`, `06`) are all
 * statements about *this* corpus: change the fixture and these are the
 * constants that have to move with it.
 */
export const SharedCorpus = {
	rustQuery: "rust",
	aliceDocId: "doc-rust-tokio",
	bobDocId: "doc-rust-borrow",
	aliceUser: "alice",
	bobUser: "bob",
	/** A user id that owns nothing — the "empty result set, not a 4xx" case. */
	unknownUser: "nobody",
	/** How many documents `q=rust` matches across the whole fixture corpus. */
	rustHitCount: 2,
} as const;

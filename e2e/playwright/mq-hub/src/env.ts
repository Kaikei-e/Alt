/**
 * Suite configuration, read once per process (the config file, global setup
 * and every worker each evaluate this module).
 *
 * Everything here is *required*. There are no defaults, deliberately:
 * CLAUDE.md rule 9 forbids warn-and-limp startup config, and a default URL is
 * exactly that failure mode wearing a test-harness hat — a suite pointed at a
 * host that does not exist reports "connection refused" on scenario 1 instead
 * of "you forgot to export BASE_URL", and a suite pointed at the *wrong* host
 * reports green. `run.sh` is the single place these are set.
 */
import { requiredEnv, runId } from "../../_shared/env.js";

export const env = {
	/**
	 * The one listener mq-hub has.
	 *
	 * `config.go` reads a single `CONNECT_PORT` (default 9500) and `main.go`
	 * mounts the Connect-RPC prefix, `/health` and `/metrics` on the same mux,
	 * so this URL is the whole service.
	 */
	baseURL: requiredEnv("BASE_URL"),

	/**
	 * A port mq-hub must **not** answer on.
	 *
	 * 9110 is the fleet's shared operator-listener port — where alt-backend,
	 * alt-harvester and alt-data-hub publish `/health` + `/metrics` after the
	 * ADR-000954 split, and where Prometheus scrapes them. mq-hub has not been
	 * split: it serves both on its RPC port. Asserting the negative pins that,
	 * so a future "give mq-hub an ops listener too" change cannot land half
	 * done, with the scrape config pointing at a port nothing binds.
	 */
	opsAbsentURL: requiredEnv("OPS_ABSENT_URL"),

	/** Unique per dispatch; embedded in stream and consumer-group names. */
	runId: runId(),
} as const;

/** The fully-qualified Connect service every procedure hangs off. */
export const MQHUB_SERVICE = "services.mqhub.v1.MQHubService";

/** Fully-qualified procedure paths, spelled once. */
export const Procedure = {
	publish: `${MQHUB_SERVICE}/Publish`,
	publishBatch: `${MQHUB_SERVICE}/PublishBatch`,
	createConsumerGroup: `${MQHUB_SERVICE}/CreateConsumerGroup`,
	getStreamInfo: `${MQHUB_SERVICE}/GetStreamInfo`,
	healthCheck: `${MQHUB_SERVICE}/HealthCheck`,
	generateTagsForArticle: `${MQHUB_SERVICE}/GenerateTagsForArticle`,
} as const;

/**
 * The four stream keys `domain.StreamKey.IsValid` recognises
 * (mq-hub/app/domain/stream.go). Unknown keys are *allowed* — the gateway only
 * logs a warning — which is what lets every test in this suite publish to a
 * stream of its own.
 */
export const CanonicalStream = {
	articles: "alt:events:articles",
	summaries: "alt:events:summaries",
	tags: "alt:events:tags",
	index: "alt:events:index",
} as const;

/**
 * `MAX_BATCH_SIZE` as the staging slice runs it.
 *
 * compose/compose.staging.yaml sets REDIS_URL, CONNECT_PORT, LOG_LEVEL,
 * REDIS_POOL_SIZE and STREAM_MAX_LEN on the mq-hub service and nothing else,
 * so `config.go`'s default of 1000 is what `publish_usecase.go` enforces.
 */
export const MAX_BATCH_SIZE = 1_000;

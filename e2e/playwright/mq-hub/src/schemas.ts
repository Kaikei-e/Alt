import { z } from "zod";
import { asInt64, int64Schema } from "../../_shared/schemas.js";

/**
 * Response envelopes, as Zod schemas.
 *
 * Hurl could only assert one JSONPath at a time, so the suite this replaces
 * checked `jsonpath "$.code" exists` on five of its fourteen scenarios — an
 * assertion satisfied by `{"code": ""}`, and by any body that happens to carry
 * a `code` key for an unrelated reason. A schema asserts the whole envelope in
 * one step, so a handler that starts answering with a different shape fails
 * here instead of silently satisfying a spot check.
 *
 * Every object schema is `passthrough()`: a contract on the fields mq-hub
 * promises, not a freeze on the ones it may add.
 */

export { connectErrorSchema } from "../../_shared/connect.js";

/**
 * A Redis Stream entry ID, `<milliseconds>-<sequence>`.
 *
 * Worth pinning rather than accepting any string: `messageId` is what a
 * publisher records to correlate its event with the stream, and an empty
 * string is exactly what `PublishResult` carries on the failure path
 * (publish_usecase.go). A bare "is a string" check would accept that.
 */
export const streamEntryIdSchema = z.string().regex(/^[0-9]+-[0-9]+$/);

/**
 * proto3 JSON renders `int64` as a decimal **string**, to survive a JSON
 * parser that would round it through a float64. `int32` stays a number.
 * That asymmetry is why `successCount` below is `z.number()` while `length`
 * is `int64Schema`.
 *
 * `int64Schema` is the fleet-wide spelling from `_shared/schemas.ts`; this
 * suite used to carry its own copy of the same regex, which is one more place
 * for the encoding to be re-derived slightly differently.
 */
export const int64StringSchema = int64Schema;

/**
 * `GET /health` — the hand-rolled handler in main.go, not a proto response.
 *
 * snake_case, and `uptime_seconds` is a real JSON number because the struct
 * is marshalled by encoding/json rather than protojson. `healthy` and
 * `redis_status` are literals rather than "a bool" / "a string": the failure
 * this catches is the 503 branch of the same handler, which answers
 * `{"healthy": false, "redis_status": "disconnected"}` with the identical set
 * of keys.
 */
export const restHealthSchema = z
	.object({
		healthy: z.literal(true),
		redis_status: z.literal("connected"),
		uptime_seconds: z.number().int().nonnegative(),
	})
	.passthrough();

/**
 * `HealthCheck` over Connect — the same three facts in protojson spelling.
 *
 * `uptimeSeconds` is **required**, which is `03-healthcheck-rpc.hurl`'s
 * `jsonpath "$.uptimeSeconds" exists` restored. Making it optional lost that
 * assertion twice over: protojson omits a zero value, and `int64(undefined)`
 * reads as 0, so a handler that stopped sending the field satisfied every
 * uptime comparison in this suite. A zero is not reachable here — compose only
 * reports mq-hub healthy once a `/health` probe passes (start_period 5s, 3s
 * interval) and setup/global-setup.ts probes again before the first test — so
 * requiring the field costs nothing and refuses the regression.
 */
export const rpcHealthSchema = z
	.object({
		healthy: z.literal(true),
		redisStatus: z.literal("connected"),
		uptimeSeconds: int64StringSchema,
	})
	.passthrough();

/**
 * The same two envelopes with their *values* un-pinned — shape only.
 *
 * Read by exactly one test, "the two health surfaces agree with each other".
 * Parsing that cross-check with the schemas above makes it a tautology:
 * `restHealthSchema` and `rpcHealthSchema` both pin `healthy` and the Redis
 * status to literals, so `true === true` and `"connected" === "connected"`
 * cannot fail once the parse has returned, and the test adds nothing over its
 * two siblings.
 *
 * The drift it exists to catch is real: main.go recomputes the string from
 * `health.Healthy` ("connected"/"disconnected") while handler.go passes
 * `usecase.HealthStatus.RedisStatus` straight through, and publish_usecase.go
 * puts `err.Error()` in that field on the unhealthy path. Two mappings, and
 * only an un-pinned comparison can observe them diverging.
 */
export const restHealthShapeSchema = z
	.object({
		healthy: z.boolean(),
		redis_status: z.string().min(1),
		uptime_seconds: z.number().int(),
	})
	.passthrough();

export const rpcHealthShapeSchema = z
	.object({
		healthy: z.boolean(),
		redisStatus: z.string().min(1),
		uptimeSeconds: int64StringSchema,
	})
	.passthrough();

/** `PublishResponse` on the success path. */
export const publishResponseSchema = z
	.object({
		success: z.literal(true),
		messageId: streamEntryIdSchema,
	})
	.passthrough();

/** One entry of `PublishBatchResponse.errors`. */
export const publishErrorSchema = z
	.object({
		index: z.number().int().nonnegative().optional(),
		errorMessage: z.string().optional(),
	})
	.passthrough();

/**
 * `PublishBatchResponse`.
 *
 * Every field is optional because protojson omits zero values, which is the
 * whole reason the Hurl suite asserted `jsonpath "$.failureCount" not exists`
 * on the happy path. Callers assert the values they seeded for; the schema's
 * job is to reject a body that is not this message at all.
 */
export const publishBatchResponseSchema = z
	.object({
		messageIds: z.array(streamEntryIdSchema).optional(),
		successCount: z.number().int().nonnegative().optional(),
		failureCount: z.number().int().nonnegative().optional(),
		errors: z.array(publishErrorSchema).optional(),
	})
	.passthrough();

/**
 * `CreateConsumerGroupResponse` on the success path.
 *
 * `message` is pinned to the exact literal handler.go writes. The Hurl suite
 * asserted `isString`, which the failure branch's "failed to create consumer
 * group" also satisfies — and that branch is returned *alongside* a Connect
 * error, so a client that ignored the error code and read the body would have
 * seen a plausible-looking message either way.
 */
export const createConsumerGroupResponseSchema = z
	.object({
		success: z.literal(true),
		message: z.literal("consumer group created"),
	})
	.passthrough();

/** One entry of `GetStreamInfoResponse.groups`. */
export const consumerGroupInfoSchema = z
	.object({
		name: z.string().min(1),
		// int64 → string, omitted at zero: a fresh group has no consumers and
		// nothing pending, so both are usually absent.
		consumers: int64StringSchema.optional(),
		pending: int64StringSchema.optional(),
		// "0-0" for a group created at the start of the stream.
		lastDeliveredId: z.string().optional(),
	})
	.passthrough();

/**
 * `GetStreamInfoResponse`.
 *
 * `firstEntryId` / `lastEntryId` carry the entry-ID regex rather than
 * `z.string()`: the driver substitutes `""` when Redis reports no entry
 * (redis_driver.go), so "is a string" would pass for a stream that exists but
 * is empty — which is the exact state a caller uses this RPC to rule out.
 */
export const streamInfoSchema = z
	.object({
		length: int64StringSchema.optional(),
		radixTreeKeys: int64StringSchema.optional(),
		radixTreeNodes: int64StringSchema.optional(),
		firstEntryId: streamEntryIdSchema.optional(),
		lastEntryId: streamEntryIdSchema.optional(),
		groups: z.array(consumerGroupInfoSchema).optional(),
	})
	.passthrough();

/**
 * Reads an int64-as-string field, treating "omitted" as the zero protojson
 * meant.
 *
 * The parse itself is `_shared/schemas.ts:asInt64`, which throws above
 * `Number.MAX_SAFE_INTEGER` instead of silently rounding — `Number.parseInt`
 * would make two different stream lengths compare equal up there.
 */
export function int64(value: string | undefined): number {
	return value === undefined ? 0 : asInt64(value);
}

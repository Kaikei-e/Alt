import { z } from "zod";

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
 * is this.
 */
export const int64StringSchema = z.string().regex(/^-?[0-9]+$/);

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
 * `uptimeSeconds` is optional because protojson omits zero values, and an
 * uptime of exactly 0 seconds is representable (if unlikely: compose waits on
 * a healthcheck with a 5s start period before run.sh even starts the suite).
 */
export const rpcHealthSchema = z
	.object({
		healthy: z.literal(true),
		redisStatus: z.literal("connected"),
		uptimeSeconds: int64StringSchema.optional(),
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

/** Reads an int64-as-string field, treating "omitted" as the zero protojson meant. */
export function int64(value: string | undefined): number {
	return value === undefined ? 0 : Number.parseInt(value, 10);
}

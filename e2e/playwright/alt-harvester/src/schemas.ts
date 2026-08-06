import { z } from "zod";

/**
 * Response envelopes, as Zod schemas.
 *
 * There is exactly one JSON body in alt-harvester's whole surface — the ops
 * `/health` envelope — so this file is small, and every line of it earns its
 * place. The Hurl suite spot-checked that body with two independent
 * `jsonpath "$.x" == "y"` assertions, which says nothing about the shape as a
 * whole: a handler that started answering
 * `{"status":"healthy","service":"alt-harvester","db":"connected"}` would have
 * satisfied both while quietly publishing a field this listener has no
 * business publishing.
 */

/**
 * `bootstrap.NewOpsHandler`'s `/health` body, pinned exactly.
 *
 * Two things are load-bearing here.
 *
 * **`service` is a literal.** All three binaries of the split are built from
 * `alt-backend/Dockerfile.backend`, differing only by `--build-arg BINARY=`,
 * and they all serve an identical-looking `/health` on an identical `:9110`.
 * The service name is the *only* thing in the response that tells them apart,
 * so it is the only thing that can catch a slice that accidentally built
 * `BINARY=backend` and tagged it `alt-alt-harvester`. Every other assertion in
 * this suite would still pass against the wrong binary right up until the
 * absent-surface probes — and those would fail with "the user API is here",
 * which reads like a regression rather than like a build mistake.
 *
 * **The object is `strict()`, against the fleet default of `passthrough()`.**
 * This listener authenticates nobody (`LogOpsWiring` logs `auth=none`) and is
 * reachable by every container on the staging network, so a field added here
 * is a field published to anything that can route to the container. `strict()`
 * makes that a decision someone has to take deliberately, in a diff that also
 * touches this line.
 */
export const opsHealthSchema = z
	.object({
		status: z.literal("healthy"),
		service: z.literal("alt-harvester"),
	})
	.strict();

/** The same body, loosened, for the smoke path where the point is liveness. */
export const opsHealthSmokeSchema = z
	.object({
		status: z.literal("healthy"),
		service: z.string().min(1),
	})
	.passthrough();

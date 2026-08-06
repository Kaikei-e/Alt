/**
 * Every Connect procedure the mux is supposed to carry, and every one it is
 * supposed to refuse.
 *
 * `connect/server.go`'s `SetupConnectHandlers` mounts exactly two services:
 * `MorningLetterService` (one streaming procedure) and `AugurService` (one
 * streaming plus four unary). Keeping the list here rather than inline in the
 * spec means the registration assertions and the topology assertions are
 * written against the same enumeration, so a service added to `server.go`
 * without a corresponding entry shows up as a gap in one place.
 */

/** Unary AugurService procedures — reachable with the JSON codec. */
export const AUGUR_UNARY = {
	retrieveContext: "alt.augur.v2.AugurService/RetrieveContext",
	listConversations: "alt.augur.v2.AugurService/ListConversations",
	getConversation: "alt.augur.v2.AugurService/GetConversation",
	deleteConversation: "alt.augur.v2.AugurService/DeleteConversation",
} as const;

/**
 * Server-streaming procedures.
 *
 * These cannot be probed with `Content-Type: application/json`: connect-go
 * registers the unary JSON content type only for unary procedures, so a
 * streaming procedure answers **415 Unsupported Media Type** with an
 * `Accept-Post` header listing `application/connect+json`. That is still a
 * *mounted* answer — 404 remains the wiring failure — which is why they get
 * their own assertion rather than being folded in with the unary ones.
 */
export const STREAMING = {
	augurStreamChat: "alt.augur.v2.AugurService/StreamChat",
	morningLetterStreamChat: "alt.morning_letter.v2.MorningLetterService/StreamChat",
} as const;

/**
 * Procedures that must **not** resolve on this mux.
 *
 * `MorningLetterReadService` is declared in the same proto file as
 * `MorningLetterService` (proto/alt/morning_letter/v2), and the two names
 * differ by one word. It is implemented by alt-backend, not here —
 * `SetupConnectHandlers` mounts only `NewMorningLetterServiceHandler`. A future
 * "while I'm here" registration would give rag-orchestrator a second, unowned
 * read surface backed by no letter store at all, so the absence is pinned.
 *
 * `AugurService/NoSuchProcedure` is the control: an unknown procedure under a
 * *known* service prefix, which connect-go's generated mux hands to
 * `http.NotFound`. Together with the mounted assertions it proves the 404
 * discriminator actually discriminates.
 */
export const ABSENT = [
	"alt.morning_letter.v2.MorningLetterReadService/GetLatestLetter",
	"alt.morning_letter.v2.MorningLetterReadService/GetLetterByDate",
	"alt.augur.v2.AugurService/NoSuchProcedure",
] as const;

package usecase

import "errors"

// Fallback codes name, in stable machine-readable form, why a turn produced no
// answer. They travel as the payload of a StreamEventKindFallback event and
// reach the client as fallback_code, where they decide which explanation the
// reader is shown.
//
// Free-form error text cannot serve that purpose. When the payload was the
// underlying error string, the client had nothing to match on but substrings,
// so a dead LLM, an expired stream, an unindexed article and a genuinely vague
// question all rendered as the same sentence: "I couldn't find enough
// information to answer that properly. Please try a more specific question."
// That sentence is advice the reader cannot act on when the cause is ours, and
// for an unindexed article no rephrasing can ever succeed.
//
// Codes are additive: a client that does not recognise one must fall back to a
// generic message rather than treat it as an error.
const (
	// FallbackCodeArticleNotIndexed — the article this question is scoped to is
	// not in the RAG index. Rephrasing cannot help; the article has to be indexed.
	FallbackCodeArticleNotIndexed = "ARTICLE_NOT_INDEXED"
	// FallbackCodeRetrievalEmpty — retrieval ran and matched nothing.
	FallbackCodeRetrievalEmpty = "RETRIEVAL_EMPTY"
	// FallbackCodeRetrievalFailed — retrieval itself failed (search backend,
	// embedder, prompt assembly). Ours, not the reader's.
	FallbackCodeRetrievalFailed = "RETRIEVAL_FAILED"
	// FallbackCodeRelevanceLow — context was found but judged too weak to answer from.
	FallbackCodeRelevanceLow = "RELEVANCE_INSUFFICIENT"
	// FallbackCodeLLMUnavailable — the generation backend could not be reached.
	FallbackCodeLLMUnavailable = "LLM_UNAVAILABLE"
	// FallbackCodeLLMStreamFailed — generation started, then the stream broke.
	FallbackCodeLLMStreamFailed = "LLM_STREAM_FAILED"
	// FallbackCodeLLMNoOutput — the stream completed without producing any tokens.
	FallbackCodeLLMNoOutput = "LLM_NO_OUTPUT"
	// FallbackCodeValidationFailed — the model's output could not be parsed or validated.
	FallbackCodeValidationFailed = "VALIDATION_FAILED"
	// FallbackCodeGenerationFailed — generation failed after the retry stage.
	FallbackCodeGenerationFailed = "GENERATION_FAILED"
	// FallbackCodeAnswerDeclined — the model itself declared it could not answer
	// from the given context. The only code that genuinely reflects the question.
	FallbackCodeAnswerDeclined = "ANSWER_DECLINED"
	// FallbackCodeAnswerRejected — an answer was produced but failed our quality gate.
	FallbackCodeAnswerRejected = "ANSWER_REJECTED"
	// FallbackCodeCausalTrailWeak — a causal-explanation answer was rejected
	// because the evidence trail was not consistent enough to assert a cause.
	// Distinct from ANSWER_REJECTED because the reader is owed a different
	// explanation: the sources exist, they just do not join up.
	FallbackCodeCausalTrailWeak = "CAUSAL_TRAIL_INSUFFICIENT"
)

// Sentinel errors for the retrieval failures that need to be told apart at the
// stream boundary. They exist so classifyRetrievalFallback can use errors.Is
// instead of matching on message text, which silently reclassifies every one of
// them the moment someone rewords an error.
var (
	// ErrNoContextRetrieved reports that retrieval completed and returned nothing.
	ErrNoContextRetrieved = errors.New("no context returned from retrieval")
	// ErrRelevanceInsufficient reports that retrieved context was judged too weak.
	ErrRelevanceInsufficient = errors.New("retrieval quality insufficient: context relevance too low")
)

// classifyRetrievalFallback maps a prompt-build failure to the code that tells
// the reader what actually went wrong.
func classifyRetrievalFallback(err error) string {
	switch {
	case errors.Is(err, ErrArticleNotIndexed):
		return FallbackCodeArticleNotIndexed
	case errors.Is(err, ErrRelevanceInsufficient):
		return FallbackCodeRelevanceLow
	case errors.Is(err, ErrNoContextRetrieved):
		return FallbackCodeRetrievalEmpty
	default:
		return FallbackCodeRetrievalFailed
	}
}

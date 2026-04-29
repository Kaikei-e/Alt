package codec

import (
	"encoding/json"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
)

// staticInterfaceCheck verifies both codecs satisfy connect.Codec at
// compile time. If the connect API drifts, the build fails here rather
// than silently at runtime.
var (
	_ connect.Codec = EmitUnpopulatedJSONCodec{}
	_ connect.Codec = EmitUnpopulatedJSONCharsetUTF8Codec{}
)

func TestEmitUnpopulatedJSONCodec_Name(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "json", EmitUnpopulatedJSONCodec{}.Name())
	assert.Equal(t, "json; charset=utf-8", EmitUnpopulatedJSONCharsetUTF8Codec{}.Name())
}

// TestEmitUnpopulatedJSONCodec_MarshalEmitsZeroValuedScalars locks the
// regression that motivated this codec: the Hurl boundary contract on
// EmitArticleUrlBackfill asserts every counter is present in the JSON
// response even when its proto3 default value is zero. The default
// protojson MarshalOptions{} drops those keys, which is what we are
// fixing here.
func TestEmitUnpopulatedJSONCodec_MarshalEmitsZeroValuedScalars(t *testing.T) {
	t.Parallel()

	resp := &knowledgehomev1.EmitArticleUrlBackfillResponse{}

	data, err := EmitUnpopulatedJSONCodec{}.Marshal(resp)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(data, &got))

	for _, key := range []string{
		"articlesScanned",
		"eventsAppended",
		"skippedBlockedScheme",
		"skippedDuplicate",
		"moreRemaining",
	} {
		_, ok := got[key]
		assert.Truef(t, ok, "expected JSON key %q to be present even at zero value, got %v", key, got)
	}

	assert.Equal(t, float64(0), got["articlesScanned"])
	assert.Equal(t, float64(0), got["eventsAppended"])
	assert.Equal(t, float64(0), got["skippedBlockedScheme"])
	assert.Equal(t, float64(0), got["skippedDuplicate"])
	assert.Equal(t, false, got["moreRemaining"])
}

func TestEmitUnpopulatedJSONCodec_MarshalNonProtoFails(t *testing.T) {
	t.Parallel()

	_, err := EmitUnpopulatedJSONCodec{}.Marshal(struct{ Name string }{Name: "x"})
	require.Error(t, err)
}

// TestEmitUnpopulatedJSONCodec_UnmarshalDiscardsUnknown ensures forward
// compatibility: a request envelope that grew a field on the client
// must still decode on the server.
func TestEmitUnpopulatedJSONCodec_UnmarshalDiscardsUnknown(t *testing.T) {
	t.Parallel()

	body := []byte(`{"maxArticles": 12, "dryRun": true, "futureField": "ignored"}`)

	req := &knowledgehomev1.EmitArticleUrlBackfillRequest{}
	require.NoError(t, EmitUnpopulatedJSONCodec{}.Unmarshal(body, req))

	assert.Equal(t, int32(12), req.MaxArticles)
	assert.True(t, req.DryRun)
}

func TestEmitUnpopulatedJSONCodec_UnmarshalNonProtoFails(t *testing.T) {
	t.Parallel()

	var dst struct{ Name string }
	err := EmitUnpopulatedJSONCodec{}.Unmarshal([]byte(`{"name":"x"}`), &dst)
	require.Error(t, err)
}

// TestEmitUnpopulatedJSONCharsetUTF8Codec_DelegatesToBaseImplementation
// confirms the charset variant is not a separate implementation — it
// shares marshal/unmarshal semantics so request and response paths
// behave identically regardless of the Content-Type the client sent.
func TestEmitUnpopulatedJSONCharsetUTF8Codec_DelegatesToBaseImplementation(t *testing.T) {
	t.Parallel()

	resp := &knowledgehomev1.EmitArticleUrlBackfillResponse{}

	base, err := EmitUnpopulatedJSONCodec{}.Marshal(resp)
	require.NoError(t, err)
	charset, err := EmitUnpopulatedJSONCharsetUTF8Codec{}.Marshal(resp)
	require.NoError(t, err)
	assert.JSONEq(t, string(base), string(charset))
}

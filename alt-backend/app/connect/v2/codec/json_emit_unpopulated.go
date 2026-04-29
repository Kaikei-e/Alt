// Package codec hosts custom connect.Codec implementations used by the
// alt-backend Connect-RPC server.
//
// The default JSON codec shipped with connectrpc.com/connect serializes
// proto3 messages via protojson with a zero-valued MarshalOptions{},
// which omits scalar fields holding their proto3 default value. The
// admin-side Hurl boundary contracts (e.g.
// e2e/hurl/alt-backend/72-admin-emit-article-url-backfill.hurl) assert
// that every field of the response envelope is present even at its
// default value, so a future refactor that drops one of them is caught
// at the wire boundary. This codec emits unpopulated values for every
// field so those contracts hold.
package codec

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// EmitUnpopulatedJSONCodec implements connect.Codec for the bare "json"
// content sub-type. Marshal uses protojson with EmitUnpopulated=true so
// proto3 default-valued scalars (int32 = 0, bool = false, string = "")
// remain present in the JSON output. Unmarshal sets DiscardUnknown=true
// so a request envelope that grew a field on the client still decodes
// on the server.
type EmitUnpopulatedJSONCodec struct{}

func (EmitUnpopulatedJSONCodec) Name() string { return "json" }

func (EmitUnpopulatedJSONCodec) Marshal(v any) ([]byte, error) {
	msg, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("emit-unpopulated json codec: expected proto.Message, got %T", v)
	}
	return protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
}

func (EmitUnpopulatedJSONCodec) Unmarshal(data []byte, v any) error {
	msg, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("emit-unpopulated json codec: expected proto.Message, got %T", v)
	}
	return protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(data, msg)
}

// EmitUnpopulatedJSONCharsetUTF8Codec mirrors EmitUnpopulatedJSONCodec
// under the "json; charset=utf-8" name. Connect-RPC selects a codec by
// exact name match against the request Content-Type sub-type, so both
// names must be registered for the override to apply when a client
// includes the charset parameter.
type EmitUnpopulatedJSONCharsetUTF8Codec struct{}

func (EmitUnpopulatedJSONCharsetUTF8Codec) Name() string { return "json; charset=utf-8" }

func (EmitUnpopulatedJSONCharsetUTF8Codec) Marshal(v any) ([]byte, error) {
	return EmitUnpopulatedJSONCodec{}.Marshal(v)
}

func (EmitUnpopulatedJSONCharsetUTF8Codec) Unmarshal(data []byte, v any) error {
	return EmitUnpopulatedJSONCodec{}.Unmarshal(data, v)
}

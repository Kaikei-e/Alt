# StreamRunProgress is server-streaming and unimplemented. Connect's
# unary HTTP/JSON path returns 415 for streaming RPCs because the
# server only accepts the framed `application/connect+json` content
# type for streams. With the streaming content-type, Connect-Python
# returns HTTP 200 and embeds the unimplemented error inside the
# stream's end-frame envelope (5-byte prefix + JSON), so the
# assertion is on the body containing "unimplemented" rather than
# a top-level JSON code.
POST {{base_url}}/alt.acolyte.v1.AcolyteService/StreamRunProgress
Content-Type: application/connect+json
Connect-Protocol-Version: 1
{
  "runId": "00000000-0000-0000-0000-000000000000"
}
HTTP 200
[Asserts]
header "content-type" contains "application/connect+json"
body contains "unimplemented"

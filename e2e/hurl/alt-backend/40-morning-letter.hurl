# Morning letter — dispatches to recap-worker (HTTP/REST). The deps-stub
# alias `recap-worker` serves a canned morning update payload.

GET {{base_url}}/v1/morning-letter/updates
X-Alt-Backend-Token: {{jwt}}
HTTP *
[Asserts]
status >= 200
status < 500

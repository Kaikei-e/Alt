#!/usr/bin/env python3
# Generates PublishBatchRequest JSON with 1001 events to trip MAX_BATCH_SIZE.
# Invoked by e2e/hurl/mq-hub/run.sh; output is gitignored.
import json
import sys

OUT = sys.argv[1] if len(sys.argv) > 1 else "batch-oversize.json"
EVENT_COUNT = 1001

events = []
for i in range(EVENT_COUNT):
    events.append({
        "eventId": f"hurl-e2e-oversize-00000000-0000-4000-8000-{i:012d}",
        "eventType": "ArticleCreated",
        "source": "e2e-hurl",
        "createdAt": "2026-04-17T12:03:00Z",
        "payload": "",
        "metadata": {},
    })

with open(OUT, "w") as f:
    json.dump({"stream": "alt:events:articles", "events": events}, f)
print(f"wrote {EVENT_COUNT} events to {OUT}", file=sys.stderr)

# AppendRecallSignal — pure write, empty response. item_key is a TEXT
# aggregate reference (matches knowledge_home_items.item_key format).
POST {{base_url}}/services.sovereign.v1.KnowledgeSovereignService/AppendRecallSignal
Content-Type: application/json
Connect-Protocol-Version: 1
{
  "signal": {
    "signalId": "{{signal_id}}",
    "userId": "{{user_id}}",
    "itemKey": "article:hurl-art-{{run_id}}",
    "signalType": "view",
    "signalStrength": 1.0,
    "occurredAt": "{{occurred_at}}",
    "payload": "e30="
  }
}
HTTP 200

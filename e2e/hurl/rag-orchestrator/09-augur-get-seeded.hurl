# GetConversation against the seeded id returns both messages in insertion
# order (created_at ASC per idx_augur_messages_conversation_created).
POST {{connect_url}}/alt.augur.v2.AugurService/GetConversation
Content-Type: application/json
Connect-Protocol-Version: 1
X-Alt-User-Id: {{user_id}}
{
  "id": "{{conversation_id}}"
}
HTTP 200
[Asserts]
jsonpath "$.id" == "{{conversation_id}}"
jsonpath "$.title" == "E2E seed conversation"
jsonpath "$.messages" count == 2
jsonpath "$.messages[0].role" == "user"
jsonpath "$.messages[0].content" == "What did the RSS reader log yesterday?"
jsonpath "$.messages[1].role" == "assistant"
jsonpath "$.messages[1].content" == "Seeded assistant reply for E2E."

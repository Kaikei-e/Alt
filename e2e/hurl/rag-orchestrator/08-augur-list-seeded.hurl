# After run.sh seeds augur_conversations + augur_messages via psql, the
# seeded user's ListConversations must surface exactly one conversation
# matching our fixture id with two messages (user + assistant).
POST {{connect_url}}/alt.augur.v2.AugurService/ListConversations
Content-Type: application/json
Connect-Protocol-Version: 1
X-Alt-User-Id: {{user_id}}
{}
HTTP 200
[Asserts]
jsonpath "$.conversations" count == 1
jsonpath "$.conversations[0].id" == "{{conversation_id}}"
jsonpath "$.conversations[0].title" == "E2E seed conversation"
jsonpath "$.conversations[0].messageCount" == 2
jsonpath "$.conversations[0].lastMessagePreview" == "Seeded assistant reply for E2E."

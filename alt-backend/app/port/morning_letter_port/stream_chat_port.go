package morning_letter_port

import (
	"context"

	"connectrpc.com/connect"

	morningletterv2 "alt/gen/proto/alt/morning_letter/v2"
)

// StreamChatPort defines the interface for streaming morning letter chat with rag-orchestrator.
type StreamChatPort interface {
	StreamChat(ctx context.Context, messages []*morningletterv2.ChatMessage, withinHours int32) (*connect.ServerStreamForClient[morningletterv2.StreamChatEvent], error)
}

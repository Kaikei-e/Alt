package rag_stream_port

import (
	"context"

	augurv2 "alt/gen/proto/alt/augur/v2"

	"connectrpc.com/connect"
)

// RagStreamPort defines the interface for streaming RAG chat.
// This port abstracts the Connect-RPC communication with rag-orchestrator.
type RagStreamPort interface {
	// StreamChat opens a streaming chat connection to the RAG service.
	// Returns a server stream that yields StreamChatResponse messages.
	StreamChat(
		ctx context.Context,
		req *connect.Request[augurv2.StreamChatRequest],
	) (*connect.ServerStreamForClient[augurv2.StreamChatResponse], error)
}

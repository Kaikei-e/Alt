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

	// ListConversations returns the caller's Ask Augur chat history.
	ListConversations(
		ctx context.Context,
		req *connect.Request[augurv2.ListConversationsRequest],
	) (*connect.Response[augurv2.ListConversationsResponse], error)

	// GetConversation returns every message in a single conversation.
	GetConversation(
		ctx context.Context,
		req *connect.Request[augurv2.GetConversationRequest],
	) (*connect.Response[augurv2.GetConversationResponse], error)

	// DeleteConversation removes a conversation and its messages.
	DeleteConversation(
		ctx context.Context,
		req *connect.Request[augurv2.DeleteConversationRequest],
	) (*connect.Response[augurv2.DeleteConversationResponse], error)
}

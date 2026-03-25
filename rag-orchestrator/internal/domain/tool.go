package domain

import "context"

// Tool defines a callable tool for the Agentic RAG system.
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, params map[string]string) (*ToolResult, error)
}

// ToolResult represents the output of a tool execution.
type ToolResult struct {
	Data    string // Text or JSON data
	Success bool
	Error   string
}

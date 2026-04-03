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
	ToolName string // Name of the tool that produced this result
	Data     string // Text or JSON data
	Success  bool
	Error    string
}

// ToolDescriptor provides metadata about a tool for the LLM planner.
type ToolDescriptor struct {
	Name        string
	Description string
}

// ToolPlan represents an LLM-generated plan for which tools to call and in what order.
type ToolPlan struct {
	Steps []ToolStep `json:"steps"`
}

// ToolStep represents a single step in a tool execution plan.
type ToolStep struct {
	ToolName  string            `json:"tool_name"`
	Params    map[string]string `json:"params"`
	DependsOn []int             `json:"depends_on,omitempty"` // indices of prerequisite steps
}

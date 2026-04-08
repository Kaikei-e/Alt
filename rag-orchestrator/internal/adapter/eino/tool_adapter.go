package eino

import (
	"context"
	"encoding/json"
	"fmt"

	"rag-orchestrator/internal/domain"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ToolAdapter wraps a domain.Tool as an Eino tool.BaseTool.
// This bridges the existing tool implementations with Eino's ChatModelAgent.
type ToolAdapter struct {
	domainTool domain.Tool
	info       *schema.ToolInfo
}

// WrapDomainTool adapts a domain.Tool to Eino's tool interface.
func WrapDomainTool(t domain.Tool) *ToolAdapter {
	return &ToolAdapter{
		domainTool: t,
		info: &schema.ToolInfo{
			Name: t.Name(),
			Desc: t.Description(),
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"query": {
					Type: schema.String,
					Desc: "Search query or input for the tool",
				},
			}),
		},
	}
}

// Info returns the tool schema information.
func (a *ToolAdapter) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return a.info, nil
}

// InvokableRun executes the tool with the given JSON input string.
func (a *ToolAdapter) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args map[string]string
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		// Try to use the raw input as query
		args = map[string]string{"query": argumentsInJSON}
	}

	result, err := a.domainTool.Execute(ctx, args)
	if err != nil {
		return "", fmt.Errorf("tool %s execution failed: %w", a.domainTool.Name(), err)
	}
	if result == nil {
		return "", nil
	}
	return result.Data, nil
}

// compile-time check
var _ tool.InvokableTool = (*ToolAdapter)(nil)

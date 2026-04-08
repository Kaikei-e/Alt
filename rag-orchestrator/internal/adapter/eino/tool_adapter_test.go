package eino

import (
	"context"
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDomainTool struct {
	name        string
	description string
	executeFunc func(ctx context.Context, args map[string]string) (*domain.ToolResult, error)
}

func (m *mockDomainTool) Name() string        { return m.name }
func (m *mockDomainTool) Description() string { return m.description }
func (m *mockDomainTool) Execute(ctx context.Context, args map[string]string) (*domain.ToolResult, error) {
	return m.executeFunc(ctx, args)
}

func TestWrapDomainTool_Info(t *testing.T) {
	mock := &mockDomainTool{name: "test_tool", description: "A test tool"}
	adapter := WrapDomainTool(mock)

	info, err := adapter.Info(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test_tool", info.Name)
	assert.Equal(t, "A test tool", info.Desc)
}

func TestWrapDomainTool_InvokableRun_JSONArgs(t *testing.T) {
	mock := &mockDomainTool{
		name: "search",
		executeFunc: func(ctx context.Context, args map[string]string) (*domain.ToolResult, error) {
			return &domain.ToolResult{Data: "result for: " + args["query"], Success: true}, nil
		},
	}
	adapter := WrapDomainTool(mock)

	result, err := adapter.InvokableRun(context.Background(), `{"query":"test query"}`)
	require.NoError(t, err)
	assert.Equal(t, "result for: test query", result)
}

func TestWrapDomainTool_InvokableRun_RawString(t *testing.T) {
	mock := &mockDomainTool{
		name: "search",
		executeFunc: func(ctx context.Context, args map[string]string) (*domain.ToolResult, error) {
			return &domain.ToolResult{Data: "result for: " + args["query"], Success: true}, nil
		},
	}
	adapter := WrapDomainTool(mock)

	result, err := adapter.InvokableRun(context.Background(), "raw query text")
	require.NoError(t, err)
	assert.Equal(t, "result for: raw query text", result)
}

func TestWrapDomainTool_UsesToolSpecificSchema(t *testing.T) {
	adapter := WrapDomainTool(&mockDomainTool{name: "tag_cloud_explore", description: "desc"})
	info, err := adapter.Info(context.Background())
	require.NoError(t, err)
	require.NotNil(t, info.ParamsOneOf)

	schema, err := info.ParamsOneOf.ToJSONSchema()
	require.NoError(t, err)
	_, ok := schema.Properties.Get("topic")
	require.True(t, ok)
	assert.Equal(t, []string{"topic"}, schema.Required)
}

func TestDefaultToolArgName(t *testing.T) {
	assert.Equal(t, "topic", defaultToolArgName("tag_cloud_explore"))
	assert.Equal(t, "tag_name", defaultToolArgName("articles_by_tag"))
	assert.Equal(t, "query", defaultToolArgName("tag_search"))
}

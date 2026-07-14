package knowledge_user_event_port

import (
	"alt/domain"
	"context"
)

// AppendKnowledgeUserEventPort appends user interaction events.
type AppendKnowledgeUserEventPort interface {
	AppendKnowledgeUserEvent(ctx context.Context, event domain.KnowledgeUserEvent) error
}

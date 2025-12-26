package answer_chat_usecase

import (
	"alt/port/rag_integration_port"
	"context"
)

type AnswerChatUsecase interface {
	Execute(ctx context.Context, input rag_integration_port.AnswerInput) (<-chan string, error)
}

type answerChatUsecase struct {
	ragIntegration rag_integration_port.RagIntegrationPort
}

func NewAnswerChatUsecase(ragIntegration rag_integration_port.RagIntegrationPort) AnswerChatUsecase {
	return &answerChatUsecase{
		ragIntegration: ragIntegration,
	}
}

func (u *answerChatUsecase) Execute(ctx context.Context, input rag_integration_port.AnswerInput) (<-chan string, error) {
	return u.ragIntegration.Answer(ctx, input)
}
